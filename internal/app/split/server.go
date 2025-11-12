package split

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Hackathon-Apps/go-split-api/internal/app/chain"
	"github.com/Hackathon-Apps/go-split-api/internal/app/config"
	"github.com/Hackathon-Apps/go-split-api/internal/app/storage"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/ton"
)

const (
	WsURL                       = "wss://tonapi.io/v2/websocket"
	TonCenterGetTransactionsURL = "https://toncenter.com/api/v2/getTransactions"
	billAutoTimeoutTTL          = 10 * time.Minute
)

var feeCollectorAddr string

type Server struct {
	configuration *config.Configuration
	logger        *logrus.Logger
	router        *mux.Router
	db            *storage.Storage
	tonApiClient  *ton.APIClient

	ws        *WsHub
	tonStream *chain.TonStream
}

func NewServer(configuration *config.Configuration, log *logrus.Logger, db *storage.Storage, api *ton.APIClient) *Server {
	ts := chain.NewTonStream(log, WsURL, configuration.TonApiToken)
	feeCollectorAddr = configuration.FeeCollectorAddress

	return &Server{
		configuration: configuration,
		logger:        log,
		router:        mux.NewRouter(),
		db:            db,
		tonApiClient:  api,
		tonStream:     ts,
		ws:            NewWSHub(),
	}
}

func (s *Server) Start() error {
	s.configureRouter()
	go s.bootstrapBillAutoTimeouts()

	s.logger.WithField("addr", s.configuration.BindAddress).Info("http: starting")
	handler := corsMiddleware(s.router)

	if err := s.tonStream.Connect(); err != nil {
		s.logger.WithError(err).Warn("tonstream: connect failed (will retry on first subscribe)")
	} else {
		s.logger.Info("tonstream: connected")
	}

	return http.ListenAndServe(s.configuration.BindAddress, handler)
}

func (s *Server) configureRouter() {
	s.router.HandleFunc("/api/healthz", s.handleHealthz()).Methods(http.MethodGet)

	s.router.HandleFunc("/api/history", s.handleHistory()).Methods(http.MethodGet)
	s.router.HandleFunc("/api/bills", s.handleCreateBill()).Methods(http.MethodPost)
	s.router.HandleFunc("/api/bills/{id}", s.handleGetBill()).Methods(http.MethodGet)
	s.router.HandleFunc("/api/bills/{id}/refund", s.handleRefundBill()).Methods(http.MethodPost)

	s.router.HandleFunc("/api/bills/{id}/transactions", s.handleCreateTransaction()).Methods(http.MethodPost)

	s.router.HandleFunc("/api/bills/{id}/ws", s.handleBillWS()).Methods(http.MethodGet)
}

func (s *Server) handleBillWS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		billID, err := uuidFromVars(mux.Vars(r), "id")
		if err != nil {
			renderErr(w, http.StatusBadRequest, err.Error())
			return
		}

		s.logger.WithFields(logrus.Fields{
			"bill_id": billID.String(),
			"remote":  r.RemoteAddr,
		}).Info("ws: subscribe request")

		s.ws.subscribe(billID.String(), w, r)

		ctx := r.Context()
		bill, err := s.db.GetBillWithSuccessTransactions(ctx, billID)
		if err == nil {
			s.ws.broadcastBill(billID.String(), bill)
			s.logger.WithField("bill_id", billID.String()).Debug("ws: initial snapshot sent")
		} else {
			s.logger.WithError(err).WithField("bill_id", billID.String()).Warn("ws: failed to fetch initial snapshot")
		}
	}
}

func (s *Server) handleHealthz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderJSON(w, "ok")
	}
}

func (s *Server) handleCreateBill() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createBillRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			renderErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}
		goal := req.Goal
		if goal <= 0 {
			renderErr(w, http.StatusBadRequest, "goal must be positive int64 (nanoton)")
			return
		}
		destinationAddr := req.DestinationAddress
		if destinationAddr == "" {
			renderErr(w, http.StatusBadRequest, "addresses is required")
			return
		}

		ctx := r.Context()
		creator, err := s.walletFromHeader(r)
		if err != nil {
			renderErr(w, http.StatusBadRequest, err.Error())
			return
		}

		proxyWalletInfo, err := chain.GenerateContractInfo(s.configuration.SmartContractHex, destinationAddr, creator, feeCollectorAddr, req.Goal)
		if err != nil {
			renderErr(w, http.StatusInternalServerError, "failed to generate TON address: "+err.Error())
			return
		}

		bill, err := s.db.CreateBill(ctx, goal, creator, destinationAddr, proxyWalletInfo.TonAddress, proxyWalletInfo.StateInitHash)
		if err != nil {
			renderErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.logger.WithFields(logrus.Fields{
			"bill_id": bill.ID.String(),
			"creator": creator,
			"goal":    goal,
			"dest":    destinationAddr,
			"proxy":   bill.ProxyWallet,
		}).Info("bill: created")

		s.scheduleBillAutoTimeoutAfter(bill.ID, billAutoTimeoutTTL)

		resp := billResponse{
			ID:                 bill.ID,
			Goal:               bill.Goal,
			Collected:          bill.Collected,
			CreatorAddress:     bill.CreatorAddress,
			DestinationAddress: bill.DestinationAddress,
			Status:             bill.Status,
			CreatedAt:          bill.CreatedAt,
			EndedAt:            bill.EndedAt,
			Transactions:       bill.Transactions,
			ProxyWalletAddress: bill.ProxyWallet,
			StateInitHash:      bill.StateInitHash,
		}
		w.WriteHeader(http.StatusCreated)
		renderJSON(w, resp)
	}
}

func (s *Server) handleGetBill() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuidFromVars(mux.Vars(r), "id")
		if err != nil {
			renderErr(w, http.StatusBadRequest, err.Error())
			return
		}

		ctx := r.Context()
		bill, err := s.db.GetBillWithSuccessTransactions(ctx, id)
		if err != nil {
			renderErr(w, http.StatusNotFound, err.Error())
			return
		}

		renderJSON(w, bill)
	}
}

func (s *Server) handleRefundBill() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuidFromVars(mux.Vars(r), "id")
		if err != nil {
			renderErr(w, http.StatusBadRequest, err.Error())
			return
		}

		creator, err := s.walletFromHeader(r)
		if err != nil {
			renderErr(w, http.StatusBadRequest, err.Error())
			return
		}

		ctx := r.Context()
		bill, err := s.db.GetBillWithSuccessTransactions(ctx, id)
		if err != nil {
			renderErr(w, http.StatusNotFound, err.Error())
			return
		}

		if bill.CreatorAddress != creator {
			renderErr(w, http.StatusBadRequest, "Refund error: creator addresses mismatch")
			return
		}

		if err = s.db.UpdateBillStatus(ctx, bill.ID, storage.StatusRefunded); err != nil {
			renderErr(w, http.StatusNotFound, err.Error())
			return
		}
		s.ws.broadcastBill(bill.ID.String(), bill)

		renderJSON(w, "ok")
	}
}

func (s *Server) handleCreateTransaction() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		billID, err := uuidFromVars(mux.Vars(r), "id")
		if err != nil {
			renderErr(w, http.StatusBadRequest, err.Error())
			return
		}

		var req createTxRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			renderErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}

		sender, err := s.walletFromHeader(r)
		if req.Amount == "" || sender == "" || req.OpType == "" {
			renderErr(w, http.StatusBadRequest, "amount, sender_address and op_type are required")
			return
		}
		amount, err := parseInt64(req.Amount)
		if err != nil || amount <= 0 {
			renderErr(w, http.StatusBadRequest, "amount must be positive int64 (nanoton)")
			return
		}
		op, err := parseOpType(req.OpType)
		if err != nil {
			renderErr(w, http.StatusBadRequest, err.Error())
			return
		}

		ctx := r.Context()
		tx, err := s.db.AddTransaction(ctx, billID, amount, sender, op)
		if err != nil {
			renderErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.logger.WithFields(logrus.Fields{
			"bill_id": billID.String(),
			"tx_id":   tx.ID.String(),
			"sender":  sender,
			"amount":  amount,
			"op":      op,
		}).Info("tx: created (PENDING)")

		go s.ensureBillSubscriptionAndWatch(billID, tx.ID)

		w.WriteHeader(http.StatusCreated)
		updated, _ := s.db.GetTransaction(context.Background(), tx.ID)
		if updated != nil {
			renderJSON(w, updated)
			return
		}
		renderJSON(w, tx)
	}
}

func (s *Server) handleHistory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sender, err := s.walletFromHeader(r)
		if err != nil {
			renderErr(w, http.StatusBadRequest, err.Error())
			return
		}

		q := r.URL.Query()
		page, err := strconv.Atoi(q.Get("page"))
		if err != nil || page <= 0 {
			page = 1
		}
		pageSize, err := strconv.Atoi(q.Get("pagesize"))
		if err != nil || pageSize <= 0 {
			pageSize = 20
		}
		if pageSize > 100 {
			pageSize = 100
		}

		ctx := r.Context()
		historyItems, total, err := s.db.GetHistory(ctx, sender, page, pageSize)
		if err != nil {
			renderErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		resp := historyItemsPage{
			Page:     page,
			PageSize: pageSize,
			Total:    int(total),
			Data:     historyItems,
		}
		renderJSON(w, resp)
	}
}

func (s *Server) ensureBillSubscriptionAndWatch(billID uuid.UUID, txID uuid.UUID) {
	bill, err := s.db.GetBillWithTransactions(context.Background(), billID)
	if err != nil {
		s.logger.WithError(err).Warn("bill not found for subscribe")
		return
	}
	var pendingTx *storage.Transaction
	for i := range bill.Transactions {
		if bill.Transactions[i].ID == txID {
			pendingTx = &bill.Transactions[i]
			break
		}
	}
	if pendingTx == nil {
		s.logger.Warn("pending tx not found")
		return
	}

	addr := bill.ProxyWallet
	if addr == "" {
		s.logger.Warn("empty proxy wallet address")
		return
	}
	rawAddr := address.MustParseAddr(addr).StringRaw()
	eventCh, cancel := s.tonStream.RegisterListener(rawAddr)

	s.logger.WithFields(logrus.Fields{
		"bill_id": billID.String(),
		"tx_id":   txID.String(),
		"address": addr,
	}).Info("tonstream: subscribe start")

	if err := s.tonStream.Subscribe(addr); err != nil {
		cancel()
		s.logger.WithError(err).Warn("ton stream subscribe failed")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"bill_id": billID.String(),
		"address": addr,
	}).Info("tonstream: subscribed")

	go s.listenForTxAndFinalize(bill, *pendingTx, addr, eventCh, cancel)
}

func (s *Server) listenForTxAndFinalize(bill *storage.Bill, pending storage.Transaction, proxyAddr string, eventCh <-chan chain.TonEvent, cancel func()) {
	timeout := time.NewTimer(10 * time.Minute)
	defer timeout.Stop()

	pollTicker := time.NewTicker(3 * time.Second)
	defer pollTicker.Stop()

	s.logger.WithFields(logrus.Fields{
		"bill_id": bill.ID.String(),
		"tx_id":   pending.ID.String(),
		"address": proxyAddr,
	}).Info("watch: started")
	defer cancel()

	rawAddr := address.MustParseAddr(proxyAddr).StringRaw()
	curEvCh := eventCh
	curCancel := cancel

	for {
		select {
		case ev, ok := <-curEvCh:
			if !ok {
				s.logger.WithFields(logrus.Fields{
					"bill_id": bill.ID.String(),
					"tx_id":   pending.ID.String(),
				}).Warn("watch: listener closed (stream reconnect in progress?)")

				// перерегистрируем listener и продолжаем ждать
				if curCancel != nil {
					curCancel()
				}
				newCh, newCancel := s.tonStream.RegisterListener(rawAddr)
				curEvCh = newCh
				curCancel = newCancel
				_ = s.tonStream.Subscribe(rawAddr) // на случай, если подписки ещё нет после реконнекта
				continue
			}

			s.logger.WithFields(logrus.Fields{
				"bill_id": bill.ID.String(),
				"tx_hash": ev.TxHash,
				"lt":      ev.LT,
			}).Info("watch: event received")

			d, err := s.fetchAndMatch(ev.LT, pending, bill)
			if err != nil {
				s.logger.WithError(err).WithFields(logrus.Fields{
					"bill_id": bill.ID.String(),
					"tx_hash": ev.TxHash,
				}).Warn("watch: fetch/match error")
				continue
			}

			if d.Matched && !d.Bounced {
				if err := s.db.UpdateTransaction(context.Background(), pending.ID, storage.StatusSuccess); err != nil {
					s.logger.WithError(err).Warn("update tx status confirmed failed")
				}
				if err := s.db.IncreaseBillCollected(context.Background(), bill.ID, d.Amount); err != nil {
					s.logger.WithError(err).Warn("increase bill collected failed")
				}
				s.logger.WithFields(logrus.Fields{
					"bill_id": bill.ID.String(),
					"tx_id":   pending.ID.String(),
					"lt":      d.LT,
					"amount":  d.Amount,
					"from":    d.From,
					"to":      d.To,
				}).Info("tx: matched -> SUCCESS")
			} else if d.Bounced {
				_ = s.db.UpdateTransaction(context.Background(), pending.ID, storage.StatusFailed)
				s.logger.WithFields(logrus.Fields{
					"bill_id": bill.ID.String(),
					"tx_id":   pending.ID.String(),
					"lt":      d.LT,
				}).Info("tx: bounced -> FAILED")
			} else {
				s.logger.WithFields(logrus.Fields{
					"bill_id": bill.ID.String(),
					"lt":      d.LT,
				}).Debug("watch: event not our tx, continue")
				continue
			}

			if updated, err := s.db.GetBillWithTransactions(context.Background(), bill.ID); err == nil {
				s.ws.broadcastBill(bill.ID.String(), updated)
				s.logger.WithField("bill_id", bill.ID.String()).Debug("ws: broadcast after update")
			}
			return

		case <-pollTicker.C:
			d, err := s.fetchAndMatchAny(pending, bill)
			if err == nil && d.Matched && !d.Bounced {
				if err := s.db.UpdateTransaction(context.Background(), pending.ID, storage.StatusSuccess); err != nil {
					s.logger.WithError(err).Warn("update tx status confirmed failed (polling)")
				}
				if err := s.db.IncreaseBillCollected(context.Background(), bill.ID, d.Amount); err != nil {
					s.logger.WithError(err).Warn("increase bill collected failed (polling)")
				}
				s.logger.WithFields(logrus.Fields{
					"bill_id": bill.ID.String(),
					"tx_id":   pending.ID.String(),
					"lt":      d.LT,
					"amount":  d.Amount,
				}).Info("tx: matched via polling -> SUCCESS")

				if updated, err := s.db.GetBillWithTransactions(context.Background(), bill.ID); err == nil {
					s.ws.broadcastBill(bill.ID.String(), updated)
				}
				return
			}

		case <-timeout.C:
			_ = s.db.UpdateTransaction(context.Background(), pending.ID, storage.StatusFailed)
			if updated, err := s.db.GetBillWithTransactions(context.Background(), bill.ID); err == nil {
				s.ws.broadcastBill(bill.ID.String(), updated)
			}
			s.logger.WithFields(logrus.Fields{
				"bill_id": bill.ID.String(),
				"tx_id":   pending.ID.String(),
			}).Warn("watch: timeout -> FAILED")
			return
		}
	}
}

func (s *Server) bootstrapBillAutoTimeouts() {
	ctx := context.Background()
	bills, err := s.db.ListBillsByStatus(ctx, storage.StatusActive)
	if err != nil {
		s.logger.WithError(err).Warn("bill: bootstrap auto-timeout failed")
		return
	}

	for _, bill := range bills {
		delay := time.Until(bill.CreatedAt.Add(billAutoTimeoutTTL))
		if delay <= 0 {
			go s.autoTimeoutBill(bill.ID)
			continue
		}

		s.scheduleBillAutoTimeoutAfter(bill.ID, delay)
	}
}

func (s *Server) scheduleBillAutoTimeoutAfter(billID uuid.UUID, delay time.Duration) {
	if delay <= 0 {
		go s.autoTimeoutBill(billID)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"bill_id": billID.String(),
		"due_in":  delay,
	}).Debug("bill: auto-timeout timer armed")

	go func(id uuid.UUID, d time.Duration) {
		timer := time.NewTimer(d)
		defer timer.Stop()

		<-timer.C
		s.autoTimeoutBill(id)
	}(billID, delay)
}

func (s *Server) autoTimeoutBill(billID uuid.UUID) {
	ctx := context.Background()
	bill, err := s.db.GetBillWithTransactions(ctx, billID)
	if err != nil {
		s.logger.WithError(err).WithField("bill_id", billID.String()).Warn("bill: auto-timeout fetch failed")
		return
	}

	if bill.Status == storage.StatusDone || bill.Status == storage.StatusTimeout {
		s.logger.WithField("bill_id", billID.String()).Debug("bill: auto-timeout skip (already finalized)")
		return
	}

	if !bill.CreatedAt.IsZero() && time.Since(bill.CreatedAt) < billAutoTimeoutTTL {
		delay := billAutoTimeoutTTL - time.Since(bill.CreatedAt)
		s.logger.WithFields(logrus.Fields{
			"bill_id":  billID.String(),
			"retry_in": delay,
		}).Debug("bill: auto-timeout rescheduled (deadline not reached)")
		s.scheduleBillAutoTimeoutAfter(billID, delay)
		return
	}

	if bill.Collected >= bill.Goal {
		s.logger.WithField("bill_id", billID.String()).Debug("bill: auto-timeout skip (goal met)")
		return
	}

	if err = s.db.UpdateBillStatus(ctx, bill.ID, storage.StatusTimeout); err != nil {
		s.logger.WithError(err).WithField("bill_id", bill.ID.String()).Warn("bill: auto-timeout update failed")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"bill_id": bill.ID.String(),
		"status":  bill.Status,
	}).Info("bill: auto-timeout status applied")

	s.ws.broadcastBill(billID.String(), bill)
}

func (s *Server) httpClient() *http.Client {
	return &http.Client{Timeout: 7 * time.Second}
}

func (s *Server) tonCenterGetTransactions(address string, limit int) (*tcGetTxResp, error) {
	start := time.Now()
	q := url.Values{}
	q.Set("address", address)
	if limit <= 0 {
		limit = 20
	}
	q.Set("limit", strconv.Itoa(limit))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, TonCenterGetTransactionsURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	if key := strings.TrimSpace(s.configuration.TonCenterApiKey); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient().Do(req)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"address": address,
			"limit":   limit,
			"ms":      time.Since(start).Milliseconds(),
		}).Warn("toncenter: request failed")
		return nil, err
	}
	defer resp.Body.Close()

	var out tcGetTxResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"status":  resp.StatusCode,
			"address": address,
			"limit":   limit,
			"ms":      time.Since(start).Milliseconds(),
		}).Warn("toncenter: decode failed")
		return nil, err
	}

	s.logger.WithFields(logrus.Fields{
		"status":  resp.StatusCode,
		"address": address,
		"limit":   limit,
		"ok":      out.Ok,
		"ms":      time.Since(start).Milliseconds(),
	}).Info("toncenter: getTransactions")

	if !out.Ok {
		return nil, fmt.Errorf("toncenter getTransactions: ok=false")
	}
	return &out, nil
}

func (s *Server) fetchAndMatch(lt uint64, pending storage.Transaction, bill *storage.Bill) (OnChainTx, error) {
	s.logger.WithFields(logrus.Fields{
		"bill_id": bill.ID.String(),
		"tx_id":   pending.ID.String(),
		"lt":      lt,
	}).Info("match: start fetch")

	onChainTx := OnChainTx{
		LT:      lt,
		To:      bill.ProxyWallet,
		Bounced: false,
	}

	pageLimit := 20
	res, err := s.tonCenterGetTransactions(bill.ProxyWallet, pageLimit)
	if err != nil {
		return onChainTx, err
	}

	for _, tx := range res.Result {
		if tx.TransactionID.LT == strconv.FormatUint(lt, 10) {
			amount, _ := strconv.ParseInt(tx.InMsg.Value, 10, 64)

			onChainTx.Amount = amount
			onChainTx.From = tx.InMsg.Source
			onChainTx.To = tx.InMsg.Destination
			onChainTx.Bounced = tx.InMsg.Bounce || tx.InMsg.Bounced

			onChainFromRaw := address.MustParseAddr(onChainTx.From).StringRaw()
			pendingFromRaw := address.MustParseAddr(pending.SenderAddress).StringRaw()

			if onChainTx.To != bill.ProxyWallet {
				s.logger.Error("proxy_wallet mismatch. OnChainTx.To:", onChainTx.To, "bill.ProxyWallet:", bill.ProxyWallet)
			}

			if onChainFromRaw != pendingFromRaw {
				s.logger.Error("from_wallet mismatch. onChainTx.From:", onChainFromRaw, "pendingFromRaw:", pendingFromRaw)
			}

			if onChainTx.Amount != pending.Amount {
				s.logger.Error("amount mismatch. onChainTx.Amount:", onChainTx.Amount, "pending.Amount:", pending.Amount)
			}

			onChainTx.Matched = strings.EqualFold(onChainTx.To, bill.ProxyWallet) &&
				strings.EqualFold(onChainFromRaw, pendingFromRaw) &&
				onChainTx.Amount >= pending.Amount

			s.logger.WithFields(logrus.Fields{
				"bill_id": bill.ID.String(),
				"lt":      onChainTx.LT,
				"from":    onChainTx.From,
				"to":      onChainTx.To,
				"amount":  onChainTx.Amount,
				"bounced": onChainTx.Bounced,
				"matched": onChainTx.Matched,
			}).Info("match: fetched")

			return onChainTx, nil
		}
	}

	s.logger.WithFields(logrus.Fields{
		"bill_id": bill.ID.String(),
		"lt":      lt,
	}).Warn("match: transaction not found in toncenter window")

	return onChainTx, fmt.Errorf("transaction %d not found for address %s", lt, bill.ProxyWallet)
}

func (s *Server) fetchAndMatchAny(pending storage.Transaction, bill *storage.Bill) (OnChainTx, error) {
	onChainTx := OnChainTx{
		To:      bill.ProxyWallet,
		Bounced: false,
	}

	res, err := s.tonCenterGetTransactions(bill.ProxyWallet, 30)
	if err != nil {
		return onChainTx, err
	}

	pendingFromRaw := address.MustParseAddr(pending.SenderAddress).StringRaw()
	targetTo := strings.ToLower(bill.ProxyWallet)

	for _, tx := range res.Result {
		if !strings.EqualFold(strings.ToLower(tx.InMsg.Destination), targetTo) {
			continue
		}
		onChainFromRaw := address.MustParseAddr(tx.InMsg.Source).StringRaw()
		if !strings.EqualFold(onChainFromRaw, pendingFromRaw) {
			continue
		}
		if tx.InMsg.Bounce || tx.InMsg.Bounced {
			continue
		}
		amount, _ := strconv.ParseInt(tx.InMsg.Value, 10, 64)
		if amount < pending.Amount {
			continue
		}

		lt, _ := strconv.ParseUint(tx.TransactionID.LT, 10, 64)
		onChainTx.LT = lt
		onChainTx.Amount = amount
		onChainTx.From = tx.InMsg.Source
		onChainTx.Matched = true
		return onChainTx, nil
	}

	return onChainTx, fmt.Errorf("pending transaction not found for proxy %s", bill.ProxyWallet)
}

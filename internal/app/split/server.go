package split

import (
	"encoding/json"
	"errors"
	"github.com/Hackathon-Apps/go-split-api/internal/app/config"
	"github.com/Hackathon-Apps/go-split-api/internal/app/storage"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/xssnick/tonutils-go/ton"
	"net/http"
	"strconv"
	"strings"
)

type Server struct {
	configuration *config.Configuration
	logger        *logrus.Logger
	router        *mux.Router
	db            *storage.Storage
	tonApiClient  *ton.APIClient
}

func NewServer(configuration *config.Configuration, log *logrus.Logger, db *storage.Storage, api *ton.APIClient) *Server {
	return &Server{
		configuration: configuration,
		logger:        log,
		router:        mux.NewRouter(),
		db:            db,
		tonApiClient:  api,
	}
}

func (s *Server) Start() error {
	s.configureRouter()

	s.logger.Info("starting server on port ", s.configuration.BindAddress)

	handler := corsMiddleware(s.router)

	return http.ListenAndServe(s.configuration.BindAddress, handler)
}

func (s *Server) configureRouter() {
	s.router.HandleFunc("/api/healthz", s.handleHealthz()).Methods(http.MethodGet)

	s.router.HandleFunc("/api/history", s.handleHistory()).Methods(http.MethodGet)
	s.router.HandleFunc("/api/bills", s.handleCreateBill()).Methods(http.MethodPost)
	s.router.HandleFunc("/api/bills/{id}", s.handleGetBill()).Methods(http.MethodGet)
	s.router.HandleFunc("/api/bills/{id}", s.handleTimeoutBill()).Methods(http.MethodPatch)

	s.router.HandleFunc("/api/bills/{id}/transactions", s.handleCreateTransaction()).Methods(http.MethodPost)
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

		proxyWalletInfo, err := GenerateContractInfo(s.configuration.SmartContractHex, destinationAddr, req.Goal)
		if err != nil {
			renderErr(w, http.StatusInternalServerError, "failed to generate TON address: "+err.Error())
			return
		}

		bill, err := s.db.CreateBill(ctx, goal, creator, destinationAddr, proxyWalletInfo.TonAddress, proxyWalletInfo.StateInitHash)
		if err != nil {
			renderErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		resp := billResponse{
			ID:                 bill.ID,
			Goal:               bill.Goal,
			Collected:          bill.Collected,
			CreatorAddress:     bill.CreatorAddress,
			DestinationAddress: bill.DestinationAddress,
			Status:             bill.Status,
			CreatedAt:          bill.CreatedAt,
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
		bill, err := s.db.GetBillWithTransactions(ctx, id)
		if err != nil {
			renderErr(w, http.StatusNotFound, err.Error())
			return
		}

		renderJSON(w, bill)
	}
}

func (s *Server) handleTimeoutBill() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuidFromVars(mux.Vars(r), "id")
		if err != nil {
			renderErr(w, http.StatusBadRequest, err.Error())
			return
		}

		ctx := r.Context()
		if err := s.db.MarkBillTimeout(ctx, id); err != nil {
			renderErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
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

		w.WriteHeader(http.StatusCreated)
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
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))

		ctx := r.Context()
		historyItems, err := s.db.GetHistory(ctx, sender, limit, offset)
		if err != nil {
			renderErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		if len(historyItems) == 0 {
			renderErr(w, http.StatusNotFound, "history items not found")
		} else {
			renderJSON(w, historyItems)
		}
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Sender-Address")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) walletFromHeader(r *http.Request) (string, error) {
	c := r.Header.Get("Sender-Address")
	w := strings.TrimSpace(c)
	if w == "" {
		return "", errors.New("empty wallet header")
	}
	if len(w) < 36 {
		return "", errors.New("invalid wallet header")
	}
	return w, nil
}

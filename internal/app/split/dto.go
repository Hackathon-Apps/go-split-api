package split

import (
	"github.com/Hackathon-Apps/go-split-api/internal/app/storage"
	"github.com/google/uuid"
	"time"
)

type createBillRequest struct {
	Goal               int64  `json:"goal"`
	DestinationAddress string `json:"destination_address"`
}

type billResponse struct {
	ID                 uuid.UUID             `json:"id"`
	Goal               int64                 `json:"goal"`
	Collected          int64                 `json:"collected"`
	CreatorAddress     string                `json:"creator_address"`
	DestinationAddress string                `json:"destination_address"`
	Status             storage.BillStatus    `json:"status"`
	CreatedAt          time.Time             `json:"created_at"`
	Transactions       []storage.Transaction `json:"transactions,omitempty"`
	ProxyWalletAddress string                `json:"proxy_wallet_address"`
	StateInitHash      string                `json:"state_init_hash"`
}

type createTxRequest struct {
	Amount string           `json:"amount"`
	OpType string           `json:"op_type"`
	Status storage.TxStatus `json:"status"`
}

type OnChainTx struct {
	LT      uint64 `json:"lt"`
	Amount  int64  `json:"amount"`
	From    string `json:"from"`
	To      string `json:"to"`
	Bounced bool   `json:"bounced"`
	Payload string `json:"payload"`
	Matched bool   `json:"matched"`
}

type tcMsgData struct {
	Type string `json:"@type"`
	Text string `json:"text,omitempty"`
	Body string `json:"body,omitempty"`
}

type tcInMsg struct {
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
	Value       string    `json:"value"`
	Message     string    `json:"message,omitempty"`
	MsgData     tcMsgData `json:"msg_data"`
	Bounce      bool      `json:"bounce,omitempty"`
	Bounced     bool      `json:"bounced,omitempty"`
}

type tcTxID struct {
	LT   string `json:"lt,omitempty"`
	ToLT string `json:"to_lt,omitempty"`
	Hash string `json:"hash"`
}

type tcTransaction struct {
	TransactionID tcTxID  `json:"transaction_id"`
	InMsg         tcInMsg `json:"in_msg"`
}

type tcPrev struct {
	LT   string `json:"lt"`
	Hash string `json:"hash"`
}

type tcGetTxResp struct {
	Ok                  bool            `json:"ok"`
	Result              []tcTransaction `json:"result"`
	PreviousTransaction *tcPrev         `json:"previous_transaction,omitempty"`
}

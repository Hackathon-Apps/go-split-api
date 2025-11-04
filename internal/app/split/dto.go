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
	Status             storage.Status        `json:"status"`
	CreatedAt          time.Time             `json:"created_at"`
	Transactions       []storage.Transaction `json:"transactions,omitempty"`
	ProxyWalletAddress string                `json:"proxy_wallet_address"`
	StateInitHash      string                `json:"state_init_hash"`
}

type createTxRequest struct {
	Amount string `json:"amount"`
	OpType string `json:"op_type"`
}

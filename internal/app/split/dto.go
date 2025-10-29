package split

import (
	"github.com/Hackathon-Apps/go-split-api/internal/app/storage"
	"github.com/google/uuid"
	"time"
)

type createBillRequest struct {
	Goal        string `json:"goal"`
	DestAddress string `json:"dest_address"`
}

type billResponse struct {
	ID           uuid.UUID             `json:"id"`
	Goal         int64                 `json:"goal"`
	Collected    int64                 `json:"collected"`
	DestAddress  string                `json:"dest_address"`
	Status       storage.Status        `json:"status"`
	CreatedAt    time.Time             `json:"created_at"`
	Transactions []storage.Transaction `json:"transactions,omitempty"`
}

type createTxRequest struct {
	Amount        string `json:"amount"`
	SenderAddress string `json:"sender_address"`
	OpType        string `json:"op_type"`
}

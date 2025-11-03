package storage

import (
	"github.com/google/uuid"
	"time"
)

type Status string

const (
	StatusActive  Status = "ACTIVE"
	StatusTimeout Status = "TIMEOUT"
	StatusDone    Status = "DONE"
)

type OpType string

const (
	OpContribute OpType = "CONTRIBUTE"
	OpTransfer   OpType = "TRANSFER"
	OpRefund     OpType = "REFUND"
)

type Bill struct {
	ID                 uuid.UUID     `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Goal               int64         `json:"goal" gorm:"not null"`
	Collected          int64         `json:"collected" gorm:"not null;default:0"`
	CreatorAddress     string        `json:"creator_address" gorm:"not null"`
	DestinationAddress string        `json:"destination_address" gorm:"not null"`
	CreatedAt          time.Time     `json:"created_at" gorm:"autoCreateTime"`
	Status             Status        `json:"status" gorm:"type:varchar(16);not null"`
	Transactions       []Transaction `json:"transactions" gorm:"foreignKey:BillID"`
	ProxyWallet        string        `json:"proxy_wallet" gorm:"not null"`
}

type Transaction struct {
	ID            uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	BillID        uuid.UUID `json:"bill_id" gorm:"type:uuid;index"`
	Amount        int64     `json:"amount" gorm:"not null"`
	SenderAddress string    `json:"sender_address" gorm:"not null"`
	CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime"`
	OpType        OpType    `json:"op_type" gorm:"type:varchar(32);not null"`
}

type HistoryItem struct {
	ID          uuid.UUID `json:"id"`
	Goal        int64     `json:"goal"`
	DestAddress string    `json:"dest_address"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

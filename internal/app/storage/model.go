package storage

import (
	"github.com/google/uuid"
	"time"
)

type BillStatus string

type TxStatus string

const (
	StatusActive  BillStatus = "ACTIVE"
	StatusTimeout BillStatus = "TIMEOUT"
	StatusDone    BillStatus = "DONE"

	StatusPending TxStatus = "PENDING"
	StatusFailed  TxStatus = "FAILED"
	StatusSuccess TxStatus = "SUCCESS"
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
	Status             BillStatus    `json:"status" gorm:"type:varchar(16);not null"`
	Transactions       []Transaction `json:"transactions" gorm:"foreignKey:BillID"`
	ProxyWallet        string        `json:"proxy_wallet" gorm:"not null"`
	StateInitHash      string        `json:"state_init_hash" gorm:"not null"`
}

type Transaction struct {
	ID            uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	BillID        uuid.UUID `json:"bill_id" gorm:"type:uuid;index"`
	Amount        int64     `json:"amount" gorm:"not null"`
	SenderAddress string    `json:"sender_address" gorm:"not null"`
	CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime"`
	OpType        OpType    `json:"op_type" gorm:"type:varchar(32);not null"`
	Status        TxStatus  `json:"status" gorm:"type:varchar(32);not null"`
}

type HistoryItem struct {
	ID                 uuid.UUID `json:"id"`
	Goal               int64     `json:"goal"`
	DestinationAddress string    `json:"destination_address"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
}

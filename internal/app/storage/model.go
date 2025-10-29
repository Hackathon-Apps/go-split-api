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
	ID           uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Goal         int64         `gorm:"not null"`
	Collected    int64         `gorm:"not null;default:0"`
	DestAddress  string        `gorm:"not null"`
	CreatedAt    time.Time     `gorm:"autoCreateTime"`
	Status       Status        `gorm:"type:varchar(16);not null"`
	Transactions []Transaction `gorm:"foreignKey:BillID"`
}

type Transaction struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	BillID        uuid.UUID `gorm:"type:uuid;index"`
	Amount        int64     `gorm:"not null"`
	SenderAddress string    `gorm:"not null"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	OpType        OpType    `gorm:"type:varchar(32);not null"`
}

type HistoryItem struct {
	ID          uuid.UUID `json:"id"`
	Goal        int64     `json:"goal"`
	DestAddress string    `json:"dest_address"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/Hackathon-Apps/go-split-api/internal/app/config"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/xssnick/tonutils-go/address"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Storage struct {
	configuration *config.Configuration
	conn          *gorm.DB
	log           *logrus.Logger
}

func Connect(cfg *config.Configuration, log *logrus.Logger) (*Storage, error) {
	storage := &Storage{configuration: cfg, log: log}

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=UTC connect_timeout=5",
		cfg.DbHost, cfg.DbUser, cfg.DbPass, cfg.DbName, cfg.DbPort,
	)

	conn, err := gorm.Open(postgres.Open(dsn))
	if err != nil {
		log.WithError(err).Error("gorm open failed")
		return nil, err
	}

	sqlDB, err := conn.DB()
	if err != nil {
		log.WithError(err).Error("get sql DB failed")
		return nil, err
	}
	for i := 0; i < 12; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		pingErr := sqlDB.PingContext(ctx)
		cancel()
		if pingErr == nil {
			break
		}
		log.WithFields(logrus.Fields{
			"attempt": i + 1, "host": cfg.DbHost, "port": cfg.DbPort,
		}).Warn("postgres not ready, retrying…")
		time.Sleep(time.Second * time.Duration(i+1))
		if i == 11 {
			log.WithError(pingErr).Error("postgres ping failed")
			return nil, pingErr
		}
	}

	storage.conn = conn
	log.WithFields(logrus.Fields{
		"host": cfg.DbHost, "port": cfg.DbPort, "user": cfg.DbUser, "db": cfg.DbName,
	}).Info("connected to PostgreSQL")
	return storage, nil
}

func (s *Storage) Conn() *gorm.DB {
	return s.conn
}

func (s *Storage) CreateBill(ctx context.Context, goal int64, creator, dest, proxyWalletAddress, stateInitHash string) (*Bill, error) {
	bill := &Bill{
		ID:                 uuid.New(),
		Goal:               goal,
		CreatorAddress:     creator,
		DestinationAddress: dest,
		Status:             StatusActive,
		ProxyWallet:        proxyWalletAddress,
		StateInitHash:      stateInitHash,
	}

	if err := s.conn.WithContext(ctx).Create(bill).Error; err != nil {
		return nil, err
	}

	return bill, nil
}

func (s *Storage) AddTransaction(ctx context.Context, billID uuid.UUID, amount int64, sender string, op OpType) (*Transaction, error) {
	tx := &Transaction{
		ID:            uuid.New(),
		BillID:        billID,
		Amount:        amount,
		SenderAddress: sender,
		OpType:        op,
		Status:        StatusPending,
	}

	if err := s.conn.WithContext(ctx).Create(tx).Error; err != nil {
		return nil, err
	}

	return tx, nil
}

func (s *Storage) GetTransaction(ctx context.Context, txId uuid.UUID) (*Transaction, error) {
	var tx Transaction
	if err := s.conn.WithContext(ctx).
		First(&tx, "id = ?", txId).Error; err != nil {
		return nil, err
	}
	return &tx, nil
}

func (s *Storage) UpdateTransaction(ctx context.Context, txId uuid.UUID, status TxStatus) error {
	return s.conn.WithContext(ctx).
		Model(&Transaction{}).
		Where("id = ?", txId).
		Update("status", status).
		Error
}

func (s *Storage) IncreaseBillCollected(ctx context.Context, billID uuid.UUID, amount int64) error {
	var bill Bill
	if err := s.conn.WithContext(ctx).
		First(&bill, "id = ?", billID).Error; err != nil {
		return err
	}
	bill.Collected += amount

	if bill.Collected >= bill.Goal {
		bill.Status = StatusDone
	}

	return s.conn.WithContext(ctx).Save(&bill).Error
}

func (s *Storage) GetBillWithTransactions(ctx context.Context, billID uuid.UUID) (*Bill, error) {
	var bill Bill
	if err := s.conn.WithContext(ctx).
		Preload("Transactions").
		First(&bill, "id = ?", billID).Error; err != nil {
		return nil, err
	}
	return &bill, nil
}

func (s *Storage) GetBillWithSuccessTransactions(ctx context.Context, billID uuid.UUID) (*Bill, error) {
	var bill Bill

	if err := s.conn.WithContext(ctx).
		Preload("Transactions", func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", StatusSuccess).Order("created_at DESC")
		}).
		First(&bill, "id = ?", billID).Error; err != nil {
		return nil, err
	}

	return &bill, nil
}

func (s *Storage) UpdateBillStatus(ctx context.Context, billID uuid.UUID, status BillStatus) error {
	return s.conn.WithContext(ctx).
		Model(&Bill{}).
		Where("id = ?", billID).
		Updates(map[string]interface{}{
			"status":   status,
			"ended_at": time.Now().UTC(),
		}).
		Error
}

func (s *Storage) ListBillsByStatus(ctx context.Context, statuses ...BillStatus) ([]Bill, error) {
	var bills []Bill
	if len(statuses) == 0 {
		return bills, nil
	}

	if err := s.conn.WithContext(ctx).
		Where("status IN ?", statuses).
		Find(&bills).Error; err != nil {
		return nil, err
	}

	return bills, nil
}

func (s *Storage) GetHistory(ctx context.Context, sender string) ([]HistoryItem, error) {
	var bills []Bill

	q := s.conn.WithContext(ctx).
		Model(&Bill{}).
		Joins("JOIN transactions t ON t.bill_id = bills.id AND t.sender_address = ? AND t.status = ?", sender, StatusSuccess).
		Preload("Transactions").
		Group("bills.id").
		Order("MAX(t.created_at) DESC")

	if err := q.Find(&bills).Error; err != nil {
		return nil, err
	}

	senderRaw := address.MustParseAddr(sender).StringRaw()
	history := make([]HistoryItem, 0, len(bills))
	for _, bill := range bills {
		txAmount := int64(0)
		for _, tx := range bill.Transactions {
			txSenderRaw := address.MustParseAddr(tx.SenderAddress).StringRaw()
			if senderRaw == txSenderRaw {
				txAmount += tx.Amount
			}
		}

		history = append(history, HistoryItem{
			ID:                 bill.ID,
			Amount:             txAmount,
			DestinationAddress: bill.DestinationAddress,
			Status:             string(bill.Status),
			CreatedAt:          bill.CreatedAt,
		})
	}

	return history, nil
}

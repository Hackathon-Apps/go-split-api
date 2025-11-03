package storage

import (
	"context"
	"fmt"
	"github.com/Hackathon-Apps/go-split-api/internal/app/config"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Storage struct {
	configuration *config.Configuration
	conn          *gorm.DB
	log           *logrus.Logger
}

func Connect(cfg *config.Configuration, log *logrus.Logger) (*Storage, error) {
	storage := &Storage{
		configuration: cfg,
		log:           log,
	}

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=UTC",
		cfg.DbHost, cfg.DbUser, cfg.DbPass, cfg.DbName, cfg.DbPort,
	)
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Error(err.Error())
	}
	storage.conn = conn

	log.Info("connected to PostgreSQL [", cfg.DbUser, "] on ", cfg.DbHost, ":", cfg.DbPort)

	return storage, nil
}

func (s *Storage) Conn() *gorm.DB {
	return s.conn
}

func (s *Storage) CreateBill(ctx context.Context, goal int64, creator, dest, proxyWallet string) (*Bill, error) {
	bill := &Bill{
		ID:                 uuid.New(),
		Goal:               goal,
		CreatorAddress:     creator,
		DestinationAddress: dest,
		Status:             StatusActive,
		ProxyWallet:        proxyWallet,
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
	}

	if err := s.conn.WithContext(ctx).Create(tx).Error; err != nil {
		return nil, err
	}

	if op == OpContribute {
		if err := s.conn.WithContext(ctx).Model(&Bill{}).
			Where("id = ?", billID).
			Update("collected", gorm.Expr("collected + ?", amount)).
			Error; err != nil {
			return tx, err
		}
	}

	// TODO: add over op_codes handling

	return tx, nil
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

func (s *Storage) MarkBillTimeout(ctx context.Context, billID uuid.UUID) error {
	return s.conn.WithContext(ctx).
		Model(&Bill{}).
		Where("id = ?", billID).
		Update("status", StatusTimeout).
		Error
}

func (s *Storage) GetHistory(ctx context.Context, sender string, limit, offset int) ([]HistoryItem, error) {
	var bills []Bill

	q := s.conn.WithContext(ctx).
		Model(&Bill{}).
		Joins("JOIN transactions t ON t.bill_id = bills.id AND t.sender_address = ?", sender).
		Preload("Transactions").
		Group("bills.id").
		Order("MAX(t.created_at) DESC")

	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}

	if err := q.Find(&bills).Error; err != nil {
		return nil, err
	}

	history := make([]HistoryItem, 0, len(bills))
	for _, bill := range bills {
		history = append(history, HistoryItem{
			ID:                 bill.ID,
			Goal:               bill.Goal,
			DestinationAddress: bill.DestinationAddress,
			Status:             string(bill.Status),
			CreatedAt:          bill.CreatedAt,
		})
	}

	return history, nil
}

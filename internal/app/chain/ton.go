package chain

import (
	"encoding/base64"
	"encoding/hex"
	"math/rand/v2"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

type ContractInfo struct {
	TonAddress    string `json:"ton_address"`
	StateInitHash string `json:"state_init_hash"`
}

func GenerateContractInfo(codeHex, receiverAddr, creatorAddr string, total int64) (*ContractInfo, error) {
	codeBOC, err := hex.DecodeString(codeHex)
	if err != nil {
		return nil, err
	}
	codeCell, err := cell.FromBOC(codeBOC)
	if err != nil {
		return nil, err
	}

	rnd := rand.Int64N(time.Now().UnixNano())
	id := uint64(rnd % 10_000)
	goal := uint64(total)

	receiver := address.MustParseAddr(receiverAddr)
	creator := address.MustParseAddr(creatorAddr)

	/*
			beginCell()
		        .storeUint(config.id, 32)
		        .storeCoins(config.goal.grams)
		        .storeAddress(config.recieverAddress)
		        .storeAddress(config.creatorAddress)
		        .storeDict(Dictionary.empty())
		        .storeUint(0, 8)
		        .endCell();
	*/
	dataCell := cell.BeginCell().
		MustStoreUInt(id, 32).
		MustStoreCoins(goal).
		MustStoreAddr(receiver).
		MustStoreAddr(creator).
		MustStoreDict(&cell.Dictionary{}).
		MustStoreUInt(0, 8).
		EndCell()

	stateInit := tlb.StateInit{
		Code: codeCell,
		Data: dataCell,
	}
	stateInitCell, err := tlb.ToCell(stateInit)
	if err != nil {
		return nil, err
	}
	bocBytes := stateInitCell.ToBOC()
	stateInitHash := base64.StdEncoding.EncodeToString(bocBytes)

	return &ContractInfo{
		TonAddress:    address.NewAddress(0, 0, stateInitCell.Hash()).String(),
		StateInitHash: stateInitHash,
	}, nil
}

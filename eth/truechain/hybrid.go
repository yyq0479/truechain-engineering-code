/*
Copyright (c) 2018 TrueChain Foundation
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package truechain

import (
	// "reflect"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	// "github.com/ethereum/go-ethereum/accounts"
	// "github.com/ethereum/go-ethereum/ethdb"
	// "github.com/ethereum/go-ethereum/event"
	// "github.com/ethereum/go-ethereum/p2p"
	// "github.com/ethereum/go-ethereum/rpc"
)


type TruePbftNode struct {
	Addr 		string 		// node ip like 127.0.0.1,the port use default
	Pubkey  	string		// 
	Privkey		string  	//
}
type TruePbftBlockHeader struct {
	Number      *big.Int       // block height out of pbft 
	GasLimit    *big.Int       // gaslimit in block include bonus tx
	GasUsed     *big.Int       // gasused in block
	Time        *big.Int       // generate time
}

type TruePbftBlock struct {
	header       *TruePbftBlockHeader
	Transactions []*types.Transaction		// raw tx（include bonus tx）
	sig		     []*string					// sign with all members
}

type StandbyInfo struct {
	nodeid		string			// the pubkey of the node(nodeid)
	coinbase	string			// the bonus address of miner
	addr		string 			
	port		int
	height		*big.Int		// block height who pow success 
	comfire		bool			// the state of the block comfire,default greater 12 like eth
}
type CommitteeMember struct {
	nodeid		string			// the pubkey of the node(nodeid)
	addr		string 			
	port		int
}
type TrueCryptoMsg struct {
	heigth		*big.Int
	msg			[]byte
	sig 		[]byte
}

func (t *TrueCryptoMsg) ToStandbyInfo() *StandbyInfo {
	return nil
}

func Validation(msg *TrueCryptoMsg) bool {
	node := msg.ToStandbyInfo()
	if node == nil {
		return false
	}
	return true
}

// type HybridConsensus interface {
// 	// main chain set node to the py-pbft
// 	MembersNodes(nodes []*TruePbftNode) error
// 	// main chain set node to the py-pbft
// 	SetTransactions(txs []*types.Transaction) error

// 	PutBlock(block *TruePbftBlock)  error

// 	ViewChange() error

// 	Start() error

// 	Stop() error
// 	// tx validation in py-pbft, Temporary slightly
// }
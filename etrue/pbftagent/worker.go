// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package pbftagent

import (
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	//"github.com/truechain/truechain-engineering-code/consensus/ethash"
	ethash "github.com/truechain/truechain-engineering-code/consensus/minerva"

)

//const (
//	resultQueueSize  = 10
//	miningLogAtDepth = 5
//
//	// txChanSize is the size of channel listening to NewTxsEvent.
//	// The number is referenced from the size of tx pool.
//	txChanSize = 4096
//	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
//	chainHeadChanSize = 10
//	// chainSideChanSize is the size of channel listening to ChainSideEvent.
//	chainSideChanSize = 10
//)

// Agent can register themself with the worker
//type Agent interface {
//	Work() chan<- *Work
//	SetReturnCh(chan<- *Result)
//	Stop()
//	Start()
//	GetHashRate() int64
//}

var work Work

// Work is the workers current environment and holds
// all of the current state information
type Work struct {
	config *params.ChainConfig
	signer types.Signer

	state     *state.StateDB // apply state changes here
	//ancestors *set.Set       // ancestor set (used for checking uncle parent validity)
	//family    *set.Set       // family set (used for checking uncle invalidity)
	//uncles    *set.Set       // uncle set
	tcount    int            // tx count in cycle
	gasPool   *core.GasPool  // available gas used to pack transactions

	Block *types.FastBlock // the new block

	header   *types.FastHeader
	txs      []*types.Transaction
	receipts []*types.Receipt

	createdAt time.Time
}

//type Result struct {
//	Work  *Work
//	Block *types.FastBlock
//}


type FastWorker struct {
	config *params.ChainConfig
	engine consensus.Engine

	mu sync.Mutex

	// update loop
	mux          *event.TypeMux
	txsCh        chan core.NewTxsEvent

	wg           sync.WaitGroup

	eth     Backend
	chain   *core.FastBlockChain
	proc    core.FastValidator
	chainDb ethdb.Database

	extra    []byte

	currentMu sync.Mutex
	current   *Work

	snapshotMu    sync.RWMutex
	snapshotBlock *types.FastBlock
	snapshotState *state.StateDB

	mining int32
	atWork int32
}

// worker is the main object which takes care of applying messages to the new state
type worker struct {
	config *params.ChainConfig
	engine consensus.Engine

	mu sync.Mutex

	// update loop
	mux          *event.TypeMux
	txsCh        chan core.NewTxsEvent
	//txsSub       event.Subscription
	//chainHeadCh  chan core.ChainHeadEvent
	//chainHeadSub event.Subscription
	//chainSideCh  chan core.ChainSideEvent
	//chainSideSub event.Subscription
	wg           sync.WaitGroup

	//agents map[Agent]struct{}
	//recv   chan *Result

	eth     Backend
	chain   *core.FastBlockChain
	proc    core.FastValidator
	chainDb ethdb.Database

	//coinbase common.Address
	extra    []byte

	currentMu sync.Mutex
	current   *Work

	snapshotMu    sync.RWMutex
	snapshotBlock *types.FastBlock
	snapshotState *state.StateDB

	//uncleMu        sync.Mutex
	//possibleUncles map[common.Hash]*types.FastBlock
	//unconfirmed *unconfirmedBlocks // set of locally mined blocks pending canonicalness confirmations

	// atomic status counters
	mining int32
	atWork int32
}


func newWorker(config *params.ChainConfig, engine consensus.Engine,eth Backend, mux *event.TypeMux) *worker {
	worker := &worker{
		config:         config,
		engine:         engine,
		eth:            eth,
		mux:            mux,
		//txsCh:          make(chan core.NewTxsEvent, txChanSize),
		//chainHeadCh:    make(chan core.ChainHeadEvent, chainHeadChanSize),
		//chainSideCh:    make(chan core.ChainSideEvent, chainSideChanSize),
		chainDb:        eth.ChainDb(),
		//recv:           make(chan *Result, resultQueueSize),
		chain:          eth.FastBlockChain(),
		proc:           eth.FastBlockChain().Validator(),
		//possibleUncles: make(map[common.Hash]*types.Block),
		//coinbase:       coinbase,
		//agents:         make(map[Agent]struct{}),
		//unconfirmed:    newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth),
	}

	// Subscribe NewTxsEvent for tx pool
	//worker.txsSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh)
	// Subscribe events for blockchain
	//worker.chainHeadSub = eth.FastBlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)
	//worker.chainSideSub = eth.FastBlockChain().SubscribeChainSideEvent(worker.chainSideCh)
	//go worker.update()

	//go worker.wait()
	worker.commitNewWork()

	return worker
}

func (self *worker) setEtherbase(addr common.Address) {
	self.mu.Lock()
	defer self.mu.Unlock()
	//self.coinbase = addr
}

func (self *worker) setExtra(extra []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.extra = extra
}

func (self *worker) pending() (*types.FastBlock, *state.StateDB) {
	if atomic.LoadInt32(&self.mining) == 0 {
		// return a snapshot to avoid contention on currentMu mutex
		self.snapshotMu.RLock()
		defer self.snapshotMu.RUnlock()
		return self.snapshotBlock, self.snapshotState.Copy()
	}

	self.currentMu.Lock()
	defer self.currentMu.Unlock()
	return self.current.Block, self.current.state.Copy()
}

func (self *worker) pendingBlock() *types.FastBlock {
	if atomic.LoadInt32(&self.mining) == 0 {
		// return a snapshot to avoid contention on currentMu mutex
		self.snapshotMu.RLock()
		defer self.snapshotMu.RUnlock()
		return self.snapshotBlock
	}

	self.currentMu.Lock()
	defer self.currentMu.Unlock()
	return self.current.Block
}

//func (self *worker) start() {
//	self.mu.Lock()
//	defer self.mu.Unlock()
//
//	atomic.StoreInt32(&self.mining, 1)
//
//	// spin up agents
//	for agent := range self.agents {
//		agent.Start()
//	}
//}

//func (self *worker) stop() {
//	self.wg.Wait()
//
//	self.mu.Lock()
//	defer self.mu.Unlock()
//	if atomic.LoadInt32(&self.mining) == 1 {
//		for agent := range self.agents {
//			agent.Stop()
//		}
//	}
//	atomic.StoreInt32(&self.mining, 0)
//	atomic.StoreInt32(&self.atWork, 0)
//}

//func (self *worker) register(agent Agent) {
//	self.mu.Lock()
//	defer self.mu.Unlock()
//	self.agents[agent] = struct{}{}
//	agent.SetReturnCh(self.recv)
//}

//func (self *worker) unregister(agent Agent) {
//	self.mu.Lock()
//	defer self.mu.Unlock()
//	delete(self.agents, agent)
//	agent.Stop()
//}

func (self *worker) update() {
	//defer self.txsSub.Unsubscribe()
	//defer self.chainHeadSub.Unsubscribe()
	//defer self.chainSideSub.Unsubscribe()

	for {
		// A real event arrived, process interesting content
		select {
		// Handle ChainHeadEvent
		//case <-self.chainHeadCh:
			//self.commitNewWork()

		// Handle ChainSideEvent
		/*case ev := <-self.chainSideCh:
			self.uncleMu.Lock()
			self.possibleUncles[ev.Block.Hash()] = ev.Block
			self.uncleMu.Unlock()*/

		// Handle NewTxsEvent
		case ev := <-self.txsCh:
			// Apply transactions to the pending state if we're not mining.
			//
			// Note all transactions received may not be continuous with transactions
			// already included in the current mining block. These transactions will
			// be automatically eliminated.
			if atomic.LoadInt32(&self.mining) == 0 {
				self.currentMu.Lock()
				txs := make(map[common.Address]types.Transactions)
				for _, tx := range ev.Txs {
					acc, _ := types.Sender(self.current.signer, tx)
					txs[acc] = append(txs[acc], tx)
				}
				txset := types.NewTransactionsByPriceAndNonce(self.current.signer, txs)
				self.current.commitTransactions(self.mux, txset, self.chain)
				self.updateSnapshot()
				self.currentMu.Unlock()
			}
			//else {
				//If we're mining, but nothing is being processed, wake on new transactions
				//if self.config.Clique != nil && self.config.Clique.Period == 0 {
				//	self.commitNewWork()
				//}
			//}

		// System stopped
		//case <-self.txsSub.Err():
		//	return
		//case <-self.chainHeadSub.Err():
		//	return
		/*case <-self.chainSideSub.Err():
			return*/
		}
	}
}

//func (self *worker) wait() {
//	for {
//		mustCommitNewWork := true
//		for result := range self.recv {
//			atomic.AddInt32(&self.atWork, -1)
//
//			if result == nil {
//				continue
//			}
//			block := result.Block
//			work := result.Work
//
//			// Update the block hash in all logs since it is now available and not when the
//			// receipt/log of individual transactions were created.
//			for _, r := range work.receipts {
//				for _, l := range r.Logs {
//					l.BlockHash = block.Hash()
//				}
//			}
//			for _, log := range work.state.Logs() {
//				log.BlockHash = block.Hash()
//			}
//			stat, err := self.chain.WriteBlockWithState(block, work.receipts, work.state)
//			if err != nil {
//				log.Error("Failed writing block to chain", "err", err)
//				continue
//			}
//			// check if canon block and write transactions
//			if stat == core.CanonStatTy {
//				// implicit by posting ChainHeadEvent
//				mustCommitNewWork = false
//			}
//			// Broadcast the block and announce chain insertion event
//			self.mux.Post(core.NewMinedBlockEvent{Block: block})
//			var (
//				events []interface{}
//				logs   = work.state.Logs()
//			)
//			events = append(events, core.ChainEvent{Block: block, Hash: block.Hash(), Logs: logs})
//			if stat == core.CanonStatTy {
//				events = append(events, core.ChainHeadEvent{Block: block})
//			}
//			self.chain.PostChainEvents(events, logs)
//
//			// Insert the block into the set of pending ones to wait for confirmations
//			//self.unconfirmed.Insert(block.NumberU64(), block.Hash())
//
//			if mustCommitNewWork {
//				self.commitNewWork()
//			}
//		}
//	}
//}

// push sends a new work task to currently live miner agents.
//func (self *worker) push(work *Work) {
//	if atomic.LoadInt32(&self.mining) != 1 {
//		return
//	}
//	for agent := range self.agents {
//		atomic.AddInt32(&self.atWork, 1)
//		if ch := agent.Work(); ch != nil {
//			ch <- work
//		}
//	}
//}

// makeCurrent creates a new environment for the current cycle.
func (self *worker) makeCurrent(parent *types.FastBlock, header *types.FastHeader) error {
	state, err := self.chain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	work := &Work{
		config:    self.config,
		signer:    types.NewEIP155Signer(self.config.ChainID),
		state:     state,
		//ancestors: set.New(),
		//family:    set.New(),
		//uncles:    set.New(),
		header:    header,
		createdAt: time.Now(),
	}

	// when 08 is processed ancestors contain 07 (quick block)
	/*for _, ancestor := range self.chain.GetBlocksFromHash(parent.Hash(), 7) {
		for _, uncle := range ancestor.Uncles() {
			work.family.Add(uncle.Hash())
		}
		work.family.Add(ancestor.Hash())
		work.ancestors.Add(ancestor.Hash())
	}*/

	// Keep track of transactions which return errors so they can be removed
	work.tcount = 0
	self.current = work
	return nil
}

func  GenerateFastBlock() (*types.FastBlock,error){
	var fastBlock  *types.FastBlock

	config :=&ethash.Config{}//etrue.DefaultConfig,
	engine := ethash.New(ethash.Config{
		CacheDir:       nil,
		CachesInMem:    config.CachesInMem,
		CachesOnDisk:   config.CachesOnDisk,
		DatasetDir:     config.DatasetDir,
		DatasetsInMem:  config.DatasetsInMem,
		DatasetsOnDisk: config.DatasetsOnDisk,
	})

	//types.NewEIP155Signer(self.config.ChainId)
	singer := types.NewEIP155Signer(big.NewInt(1))

	/*chainDb, err := CreateDB(ctx, config, "chaindata")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)*/

	self := &FastWorker{
		//config:         config,
		engine:         engine,
		//eth:            eth,
		//mux:            mux,
		//chain:          eth.BlockChain(),
		//proc:           eth.BlockChain().Validator(),
	}

	//1 准备新区块的时间属性Header.Time
	tstart := time.Now()
	parent := self.chain.CurrentBlock()

	tstamp := tstart.Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) >= 0 {
		tstamp = parent.Time().Int64() + 1
	}
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); tstamp > now+1 {
		wait := time.Duration(tstamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	//2 创建新区块的Header对象，
	num := parent.Number()
	header := &types.FastHeader{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.FastCalcGasLimit(parent),
		//Extra:      self.extra,
		Time:       big.NewInt(tstamp),
	}
	// 3 调用Engine.Prepare()函数，完成Header对象的准备。
	if err := self.engine.PrepareFast(self.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return	fastBlock,err
	}
	// 4 根据已有的Header对象，创建一个新的Work对象，并用其更新worker.current成员变量。
	// Create the current work task and check any fork transitions needed
	work := self.current

	//5 准备新区块的交易列表，来源是TxPool中那些最近加入的tx，并执行这些交易。
	pending, err := self.eth.TxPool().Pending()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return	fastBlock,err
	}
	txs := types.NewTransactionsByPriceAndNonce(singer, pending)
	work.commitTransactions(self.mux, txs, self.chain)

	// 6 对新区块“定型”，填充上Header.Root, TxHash, ReceiptHash等几个属性。
	// Create the new block to seal with the consensus engine
	if work.Block, err = self.engine.FinalizeFast(self.chain, header, work.state, work.txs, work.receipts); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return	fastBlock,err
	}
	self.updateSnapshot()
	return	fastBlock,nil
}

func (self *worker) commitNewWork() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	tstart := time.Now()
	parent := self.chain.CurrentBlock()

	tstamp := tstart.Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) >= 0 {
		tstamp = parent.Time().Int64() + 1
	}
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); tstamp > now+1 {
		wait := time.Duration(tstamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	num := parent.Number()
	header := &types.FastHeader{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.FastCalcGasLimit(parent),
		Extra:      self.extra,
		Time:       big.NewInt(tstamp),
	}

	if err := self.engine.PrepareFast(self.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return
	}

	err := self.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
	// Create the current work task and check any fork transitions needed
	work := self.current

	pending, err := self.eth.TxPool().Pending()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return
	}
	txs := types.NewTransactionsByPriceAndNonce(self.current.signer, pending)
	work.commitTransactions(self.mux, txs, self.chain)

	// Create the new block to seal with the consensus engine
	if work.Block, err = self.engine.FinalizeFast(self.chain, header, work.state, work.txs, work.receipts); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return
	}

	self.updateSnapshot()
}

//func (self *worker) commitUncle(work *Work, uncle *types.Header) error {
//	hash := uncle.Hash()
//	if work.uncles.Has(hash) {
//		return fmt.Errorf("uncle not unique")
//	}
//	if !work.ancestors.Has(uncle.ParentHash) {
//		return fmt.Errorf("uncle's parent unknown (%x)", uncle.ParentHash[0:4])
//	}
//	if work.family.Has(hash) {
//		return fmt.Errorf("uncle already in family (%x)", hash)
//	}
//	work.uncles.Add(uncle.Hash())
//	return nil
//}

func (self *FastWorker) updateSnapshot() {
	self.snapshotMu.Lock()
	defer self.snapshotMu.Unlock()

	self.snapshotBlock = types.NewFastBlock(
		self.current.header,
		self.current.txs,
		nil,
		self.current.receipts,
	)
	self.snapshotState = self.current.state.Copy()
}

func (self *worker) updateSnapshot() {
	self.snapshotMu.Lock()
	defer self.snapshotMu.Unlock()

	self.snapshotBlock = types.NewFastBlock(
		self.current.header,
		self.current.txs,
		nil,
		self.current.receipts,
	)
	self.snapshotState = self.current.state.Copy()
}

func (env *Work) commitTransactions(mux *event.TypeMux, txs *types.TransactionsByPriceAndNonce, bc *core.FastBlockChain) {
	if env.gasPool == nil {
		env.gasPool = new(core.GasPool).AddGas(env.header.GasLimit)
	}

	var coalescedLogs []*types.Log

	for {
		// If we don't have enough gas for any further transactions then we're done
		if env.gasPool.Gas() < params.TxGas {
			log.Trace("Not enough gas for further transactions", "have", env.gasPool, "want", params.TxGas)
			break
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		// Error may be ignored here. The error has already been checked
		// during transaction acceptance is the transaction pool.
		//
		// We use the eip155 signer regardless of the current hf.
		from, _ := types.Sender(env.signer, tx)
		// Check whether the tx is replay protected. If we're not in the EIP155 hf
		// phase, start ignoring the sender until we do.
		if tx.Protected() && !env.config.IsEIP155(env.header.Number) {
			log.Trace("Ignoring reply protected transaction", "hash", tx.Hash(), "eip155", env.config.EIP155Block)

			txs.Pop()
			continue
		}
		// Start executing the transaction
		env.state.Prepare(tx.Hash(), common.Hash{}, env.tcount)

		err, logs := env.commitTransaction(tx, bc,env.gasPool)
		switch err {
		case core.ErrGasLimitReached:
			// Pop the current out-of-gas transaction without shifting in the next from the account
			log.Trace("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case core.ErrNonceTooLow:
			// New head notification data race between the transaction pool and miner, shift
			log.Trace("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case core.ErrNonceTooHigh:
			// Reorg notification data race between the transaction pool and miner, skip account =
			log.Trace("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case nil:
			// Everything ok, collect the logs and shift in the next transaction from the same account
			coalescedLogs = append(coalescedLogs, logs...)
			env.tcount++
			txs.Shift()

		default:
			// Strange error, discard the transaction and get the next in line (note, the
			// nonce-too-high clause will prevent us from executing in vain).
			log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

	if len(coalescedLogs) > 0 || env.tcount > 0 {
		// make a copy, the state caches the logs and these logs get "upgraded" from pending to mined
		// logs by filling in the block hash when the block was mined by the local miner. This can
		// cause a race condition if a log was "upgraded" before the PendingLogsEvent is processed.
		cpy := make([]*types.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		go func(logs []*types.Log, tcount int) {
			if len(logs) > 0 {
				mux.Post(core.PendingLogsEvent{Logs: logs})
			}
			if tcount > 0 {
				mux.Post(core.PendingStateEvent{})
			}
		}(cpy, env.tcount)
	}
}

func (env *Work) commitTransaction(tx *types.Transaction, bc *core.FastBlockChain,  gp *core.GasPool) (error, []*types.Log) {
	snap := env.state.Snapshot()

	receipt, _, err := core.FastApplyTransaction(env.config, bc, gp, env.state, env.header, tx, &env.header.GasUsed, vm.Config{})
	if err != nil {
		env.state.RevertToSnapshot(snap)
		return err, nil
	}
	env.txs = append(env.txs, tx)
	env.receipts = append(env.receipts, receipt)

	return nil, receipt.Logs
}

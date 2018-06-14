// Copyright 2018 The go-hpb Authors
// This file is part of the go-hpb.
//
// The go-hpb is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-hpb is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-hpb. If not, see <http://www.gnu.org/licenses/>.

package synctrl

import (
	"github.com/hpb-project/go-hpb/types"
	"github.com/hpb-project/go-hpb/common"
	"math/rand"
)

type txsync struct {
	p   *peer //todo qinghua's peer
	txs []*types.Transaction
}

// syncTransactions starts sending all currently pending transactions to the given peer.
func (this *SynCtrl) syncTransactions(p *peer) {//todo qinghua's peer
	var txs types.Transactions
	pending, _ := this.txpool.Pending()//todo xinyu's
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	if len(txs) == 0 {
		return
	}
	select {
	case this.txsyncCh <- &txsync{p, txs}:
	case <-this.quitSync:
	}
}

// txsyncLoop takes care of the initial transaction sync for each new
// connection. When a new peer appears, we relay all currently pending
// transactions. In order to minimise egress bandwidth usage, we send
// the transactions in small packs to one peer at a time.
func (this *SynCtrl) txsyncLoop() {
	var (
		pending = make(map[discover.NodeID]*txsync)//todo qinghua's peer
		sending = false               // whether a send is active
		pack    = new(txsync)         // the pack that is being sent
		done    = make(chan error, 1) // result of the send
	)

	// send starts a sending a pack of transactions from the sync.
	send := func(s *txsync) {
		// Fill pack with transactions up to the target size.
		size := common.StorageSize(0)
		pack.p = s.p
		pack.txs = pack.txs[:0]
		for i := 0; i < len(s.txs) && size < txsyncPackSize; i++ {
			pack.txs = append(pack.txs, s.txs[i])
			size += s.txs[i].Size()
		}
		// Remove the transactions that will be sent.
		s.txs = s.txs[:copy(s.txs, s.txs[len(pack.txs):])]
		if len(s.txs) == 0 {
			delete(pending, s.p.ID())//todo qinghua's peer
		}
		// Send the pack in the background.
		s.p.Log().Trace("Sending batch of transactions", "count", len(pack.txs), "bytes", size)//todo qinghua's peer
		sending = true
		go func() { done <- pack.p.SendTransactions(pack.txs) }()//todo qinghua's peer
	}

	// pick chooses the next pending sync.
	pick := func() *txsync {
		if len(pending) == 0 {
			return nil
		}
		n := rand.Intn(len(pending)) + 1
		for _, s := range pending {
			if n--; n == 0 {
				return s
			}
		}
		return nil
	}

	for {
		select {
		case s := <-this.txsyncCh:
			pending[s.p.ID()] = s//todo qinghua's peer
			if !sending {
				send(s)
			}
		case err := <-done:
			sending = false
			// Stop tracking peers that cause send failures.
			if err != nil {
				pack.p.Log().Debug("Transaction send failed", "err", err)//todo qinghua's peer
				delete(pending, pack.p.ID())//todo qinghua's peer
			}
			// Schedule the next send.
			if s := pick(); s != nil {
				send(s)
			}
		case <-this.quitSync:
			return
		}
	}
}

func (this *SynCtrl) txBroadcastLoop() {
	for {
		select {
		case event := <-this.txCh:
			this.broadcastTx(event.Tx.Hash(), event.Tx)

			// Err() channel will be closed when unsubscribing.
		case <-this.txSub.Err():
			return
		}
	}
}
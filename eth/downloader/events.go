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

package downloader

import "github.com/electroneum/electroneum-sc/core/types"

type DoneEvent struct {
	Latest *types.Header
}
type StartEvent struct{}
type FailedEvent struct{ Err error }

// ProgressEvent is posted once a synchronisation attempt has made verifiable
// progress, i.e. the remote peer has actually served a valid chain head rather
// than merely advertising a high total difficulty during the handshake.
//
// It exists so that consumers which pause work while syncing (notably the
// miner, and with it the IBFT/QBFT consensus engine) can wait for evidence that
// a peer really serves the chain before stopping. An unauthenticated peer can
// trigger StartEvent for free by claiming a huge TD and then never delivering
// any headers; keying the pause off ProgressEvent instead prevents such a peer
// from stalling consensus for the request timeout on a node that has not yet
// completed a sync.
type ProgressEvent struct{}

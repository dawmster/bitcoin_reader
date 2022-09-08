package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"time"

	// "github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcd/wire"
	"github.com/tokenized/bitcoin_reader"
	"github.com/tokenized/bitcoin_reader/headers"
	"github.com/tokenized/config"
	"github.com/tokenized/logger"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/threads"

	"github.com/pkg/errors"
)

var (
	buildVersion = "unknown"
	buildDate    = "unknown"
	buildUser    = "unknown"
)

type Miko_TxProcessor struct { // implements interface bitcoin_reader.TxProcessor
	nic int
}

func (tp Miko_TxProcessor) ProcessTx(ctx context.Context, tx *wire.MsgTx) (bool, error) {
	fmt.Println("-TX-")
	// for _, value := range tx.TxIn {
	// 	fmt.Print(" %s", value.PreviousOutPoint.String())
	// }
	for _, value := range tx.TxOut {
		fmt.Print(" %v", value.LockingScript.String())
		// fmt.Print(" %s", value.LockingScript.MarshalJSON())
	}
	fmt.Println("--")

	return false, nil
}

//this is taken from tokenized/pkg/bitcoin/script.go by me. Miko.
// ScriptToStrings converts a bitcoin script into a array of text representation - for each opcode and data.
func ScriptToStrings(script bitcoin.Script) []string {
	var result []string
	buf := bytes.NewReader(script)

	for {
		item, err := bitcoin.ParseScript(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		result = append(result, item.String())
	}

	return result
}

// ScriptToScriptItems converts a bitcoin script into a array of text representation - for each opcode and data.
func ScriptToScriptItems(script bitcoin.Script) []bitcoin.ScriptItem {
	var result []bitcoin.ScriptItem
	buf := bytes.NewReader(script)

	for {
		item, err := bitcoin.ParseScript(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		result = append(result, *item)
	}

	return result
}

func (tp Miko_TxProcessor) CancelTx(ctx context.Context, txid bitcoin.Hash32) error {

	return nil
}

func (tp Miko_TxProcessor) AddTxConflict(ctx context.Context, txid, conflictTxID bitcoin.Hash32) error {
	return nil
}

func (tp Miko_TxProcessor) ConfirmTx(ctx context.Context, txid bitcoin.Hash32, blockHeight int,
	merkleProof *merkle_proof.MerkleProof) error {

	return nil
}

func (tp Miko_TxProcessor) UpdateTxChainDepth(ctx context.Context, txid bitcoin.Hash32, chainDepth uint32) error {
	return nil
}

func (tp Miko_TxProcessor) ProcessCoinbaseTx(ctx context.Context, blockHash bitcoin.Hash32, tx *wire.MsgTx) error {
	return nil
}

func main() {
	// ---------------------------------------------------------------------------------------------
	// Logging
	fmt.Println("start:")

	logPath := "./tmp/node/node.log"
	if len(logPath) > 0 {
		os.MkdirAll(path.Dir(logPath), os.ModePerm)
	}
	isDevelopment := false

	logConfig := logger.NewConfig(isDevelopment, false, logPath)

	ctx := logger.ContextWithLogConfig(context.Background(), logConfig)

	logger.Info(ctx, "Started : Application Initializing")
	defer logger.Info(ctx, "Completed")

	logger.Info(ctx, "Build %v (%v on %v)", buildVersion, buildUser, buildDate)

	// ---------------------------------------------------------------------------------------------
	// Storage

	store, err := storage.CreateStorage("standalone", "./tmp/node", 5, 100)
	if err != nil {
		logger.Fatal(ctx, "Failed to create storage : %s", err)
	}

	headers := headers.NewRepository(headers.DefaultConfig(), store)
	peers := bitcoin_reader.NewPeerRepository(store, "")

	if err := headers.Load(ctx); err != nil {
		logger.Fatal(ctx, "Failed to load headers : %s", err)
	}

	if err := peers.Load(ctx); err != nil {
		logger.Fatal(ctx, "Failed to load peers : %s", err)
	}

	if peers.Count() == 0 {
		peers.LoadSeeds(ctx, bitcoin.MainNet)
	}

	var wait sync.WaitGroup
	var stopper threads.StopCombiner

	stopper.Add(headers)

	// ---------------------------------------------------------------------------------------------
	// Node Manager (Bitcoin P2P)

	userAgent := fmt.Sprintf("/Tokenized/Spynode:Test-%s/", buildVersion)
	logger.Info(ctx, "User Agent : %s", userAgent)

	nodeConfig := &bitcoin_reader.Config{
		Network:                 bitcoin.MainNet,
		Timeout:                 config.NewDuration(time.Hour),
		ScanCount:               500,
		StartupDelay:            config.NewDuration(time.Second * 20),
		ConcurrentBlockRequests: 2,
		DesiredNodeCount:        50,
		BlockRequestDelay:       config.NewDuration(time.Second * 5),
	}
	manager := bitcoin_reader.NewNodeManager(userAgent, nodeConfig, headers, peers)
	managerThread, managerComplete := threads.NewInterruptableThreadComplete("Node Manager",
		manager.Run, &wait)
	stopper.Add(managerThread)
	// stopper.Add(blockManager)

	// processBlocksThread.SetWait(&wait)
	// processBlocksComplete := processBlocksThread.GetCompleteChannel()
	// stopper.Add(processBlocksThread)

	// ---------------------------------------------------------------------------------------------
	// Periodic

	saveThread, saveComplete := threads.NewPeriodicThreadComplete("Save",
		func(ctx context.Context) error {
			if err := headers.Clean(ctx); err != nil {
				return errors.Wrap(err, "clean headers")
			}
			if err := peers.Save(ctx); err != nil {
				return errors.Wrap(err, "save peers")
			}
			return nil
		}, 30*time.Minute, &wait)
	stopper.Add(saveThread)

	previousTime := time.Now()
	cleanTxsThread, cleanTxsComplete := threads.NewPeriodicThreadComplete("Clean Txs",
		func(ctx context.Context) error {
			if err := txManager.Clean(ctx, previousTime); err != nil {
				return errors.Wrap(err, "clean tx manager")
			}
			previousTime = time.Now()
			return nil
		}, 5*time.Minute, &wait)
	stopper.Add(cleanTxsThread)

	// ---------------------------------------------------------------------------------------------
	// Shutdown

	// Make a channel to listen for an interrupt or terminate signal from the OS. Use a buffered
	// channel because the signal package requires it.
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)

	managerThread.Start(ctx)
	// processBlocksThread.Start(ctx)
	saveThread.Start(ctx)
	cleanTxsThread.Start(ctx)
	processTxThread.Start(ctx)

	// Blocking main and waiting for shutdown.
	select {
	case <-managerComplete:
		logger.Warn(ctx, "Finished: Manager")

	case <-saveComplete:

	case <-processTxComplete:
		logger.Warn(ctx, "Finished: Process Txs")

		logger.Info(ctx, "Shutdown requested")
	}

	// Stop remaining threads
	stopper.Stop(ctx)

	// Block until goroutines finish
	waitWarning := logger.NewWaitingWarning(ctx, 3*time.Second, "Shutdown")
	wait.Wait()
	waitWarning.Cancel()

	if err := headers.Save(ctx); err != nil {
		logger.Error(ctx, "Failed to save headers : %s", err)
	}
	if err := peers.Save(ctx); err != nil {
		logger.Error(ctx, "Failed to save peers : %s", err)
	}

	if err := threads.CombineErrors(
		managerThread.Error(),
		saveThread.Error(),
		cleanTxsThread.Error(),
	); err != nil {
		logger.Error(ctx, "Failed : %s", err)
	}
}

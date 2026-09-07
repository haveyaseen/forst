package main

import (
	fmt "fmt"
	os "os"
	"strconv"
)

func main() {
	var syncSum float64 = 0
	syncSeq, syncSeqErr := forst_bridge_open_seq_legacy_generators_js_syncNumbers()
	if syncSeqErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", syncSeqErr)
			os.Exit(1)
		}
	}
	{
		_nodeIt := syncSeq
		defer _nodeIt.Close()
		var (
			_nodeStep     forstBridgeGenStep_float64
			_nodeBatch    []forstBridgeGenStep_float64
			_nodeBatchIdx int
			_nodeBatchErr error
		)
		_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
		if _nodeBatchErr != nil {
			panic(_nodeBatchErr)
		}
		_nodeBatchIdx = 0
		for {
			if _nodeBatchIdx >= len(_nodeBatch) {
				_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
				if _nodeBatchErr != nil {
					panic(_nodeBatchErr)
				}
				_nodeBatchIdx = 0
			}
			_nodeStep = _nodeBatch[_nodeBatchIdx]
			_nodeBatchIdx++
			if _nodeStep.Kind == forstBridgeGenStepDone {
				break
			}
			if _nodeStep.Kind == forstBridgeGenStepError {
				panic(_nodeStep.Message)
			}
			n := _nodeStep.Value
			syncSum = syncSum + n
		}
	}
	println("sync:" + strconv.FormatFloat(syncSum, 'f', 0, 64))
	var asyncSum float64 = 0
	asyncSeq, asyncSeqErr := forst_bridge_open_seq_legacy_generators_js_asyncNumbers()
	if asyncSeqErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", asyncSeqErr)
			os.Exit(1)
		}
	}
	{
		_nodeIt := asyncSeq
		defer _nodeIt.Close()
		var (
			_nodeStep     forstBridgeGenStep_float64
			_nodeBatch    []forstBridgeGenStep_float64
			_nodeBatchIdx int
			_nodeBatchErr error
		)
		_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
		if _nodeBatchErr != nil {
			panic(_nodeBatchErr)
		}
		_nodeBatchIdx = 0
		for {
			if _nodeBatchIdx >= len(_nodeBatch) {
				_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
				if _nodeBatchErr != nil {
					panic(_nodeBatchErr)
				}
				_nodeBatchIdx = 0
			}
			_nodeStep = _nodeBatch[_nodeBatchIdx]
			_nodeBatchIdx++
			if _nodeStep.Kind == forstBridgeGenStepDone {
				break
			}
			if _nodeStep.Kind == forstBridgeGenStepError {
				panic(_nodeStep.Message)
			}
			n := _nodeStep.Value
			asyncSum = asyncSum + n
		}
	}
	println("async:" + strconv.FormatFloat(asyncSum, 'f', 0, 64))
	var emptyCount int = 0
	emptySeq, emptySeqErr := forst_bridge_open_seq_legacy_generators_js_emptyGen()
	if emptySeqErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", emptySeqErr)
			os.Exit(1)
		}
	}
	{
		_nodeIt := emptySeq
		defer _nodeIt.Close()
		var (
			_nodeStep     forstBridgeGenStep_float64
			_nodeBatch    []forstBridgeGenStep_float64
			_nodeBatchIdx int
			_nodeBatchErr error
		)
		_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
		if _nodeBatchErr != nil {
			panic(_nodeBatchErr)
		}
		_nodeBatchIdx = 0
		for {
			if _nodeBatchIdx >= len(_nodeBatch) {
				_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
				if _nodeBatchErr != nil {
					panic(_nodeBatchErr)
				}
				_nodeBatchIdx = 0
			}
			_nodeStep = _nodeBatch[_nodeBatchIdx]
			_nodeBatchIdx++
			if _nodeStep.Kind == forstBridgeGenStepDone {
				break
			}
			if _nodeStep.Kind == forstBridgeGenStepError {
				panic(_nodeStep.Message)
			}
			emptyCount = emptyCount + 1
		}
	}
	println("empty:" + strconv.Itoa(emptyCount))
	var breakCount int = 0
	finallySeq, finallySeqErr := forst_bridge_open_seq_legacy_generators_js_withFinally()
	if finallySeqErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", finallySeqErr)
			os.Exit(1)
		}
	}
	{
		_nodeIt := finallySeq
		defer _nodeIt.Close()
		var (
			_nodeStep     forstBridgeGenStep_float64
			_nodeBatch    []forstBridgeGenStep_float64
			_nodeBatchIdx int
			_nodeBatchErr error
		)
		_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
		if _nodeBatchErr != nil {
			panic(_nodeBatchErr)
		}
		_nodeBatchIdx = 0
		for {
			if _nodeBatchIdx >= len(_nodeBatch) {
				_nodeBatch, _nodeBatchErr = _nodeIt.NextBatch(32)
				if _nodeBatchErr != nil {
					panic(_nodeBatchErr)
				}
				_nodeBatchIdx = 0
			}
			_nodeStep = _nodeBatch[_nodeBatchIdx]
			_nodeBatchIdx++
			if _nodeStep.Kind == forstBridgeGenStepDone {
				break
			}
			if _nodeStep.Kind == forstBridgeGenStepError {
				panic(_nodeStep.Message)
			}
			n := _nodeStep.Value
			breakCount = breakCount + 1
			if n == 2 {
				break
			}
		}
	}
	println("break:" + strconv.Itoa(breakCount))
}

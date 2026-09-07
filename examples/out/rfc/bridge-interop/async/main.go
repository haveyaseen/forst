package main

import (
	fmt "fmt"
	os "os"
	"strconv"
)

type T_BSiWS9EsB18 struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Id       string  `json:"id"`
}

type T_G2n6vMumV8n struct {
	Type string `json:"type"`
}

type T_NTbLJjyksQg struct {
	Echo float64 `json:"echo"`
}

func checkout(amount float64, currency string) (string, error) {
	result, resultErr := forst_bridge_callasync_legacy_payment_js_create(amount, currency)
	if resultErr != nil {
		return "", resultErr
	}
	return result.Id, nil
}

func drainEvents(userId string) (int, error) {
	seq, seqErr := forst_bridge_open_seq_legacy_events_js_subscribe(userId)
	if seqErr != nil {
		return 0, seqErr
	}
	var count int = 0
	{
		_nodeIt := seq
		defer _nodeIt.Close()
		var (
			_nodeStep     forstBridgeGenStep_t_g2n6vmumv8n
			_nodeBatch    []forstBridgeGenStep_t_g2n6vmumv8n
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
			evt := _nodeStep.Value
			forst_bridge_callasync_legacy_events_js_dispatch(evt)
			count = count + 1
		}
	}
	return count, nil
}

func echo(n float64) (string, error) {
	res, resErr := forst_bridge_callasync_legacy_payment_js_concurrentEcho(n)
	if resErr != nil {
		return "", resErr
	}
	return strconv.FormatFloat(res.Echo, 'f', 0, 64), nil
}

func main() {
	id, idErr := checkout(100, "USD")
	if idErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", idErr)
			os.Exit(1)
		}
	}
	println("payment:" + id)
	msg, msgErr := echo(7)
	if msgErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", msgErr)
			os.Exit(1)
		}
	}
	println("echo:" + msg)
	n, nErr := drainEvents("user-42")
	if nErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", nErr)
			os.Exit(1)
		}
	}
	println("events:" + strconv.Itoa(n))
}

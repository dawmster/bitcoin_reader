# Bitcoin Reader

Bitcoin reader finds peers in the Bitcoin P2P network and listens to them. It always collects all
headers with valid proof of work. If a tx processor is provided then all txs are run through it as
they are seen on the network. If a block manager is provided then all blocks are fed through it.

## Example

cmd/node/main.go

There is Miko_TxProcessor that has a method ProcessTx in line 39. It prints out Locking Script of processed transactions. Work In Progress.

To run:
- go to node/cmd
- `go run main.go  > out.txt ; tail -f out.txt`

To stop press Ctrl+C.
# Start the celestia-app
celestia-appd start --home $1 &

# Try to get the genesis hash. Usually first request returns an empty string (port is not open, curl fails), later attempts
# returns "null" if block was not yet produced.
GENESIS=
CNT=0
MAX=30
while [ "${#GENESIS}" -le 4 -a $CNT -ne $MAX ]; do
	GENESIS=$(curl -s http://127.0.0.1:26657/block?height=1 | jq '.result.block_id.hash' | tr -d '"')
	((CNT++))
	sleep 1
done

export CELESTIA_CUSTOM=test:$GENESIS
echo "$CELESTIA_CUSTOM"

celestia bridge init --node.store /home/celestia/bridge
celestia bridge start \
  --node.store "/home/celestia/bridge" --gateway \
  --core.ip 127.0.0.1 \
  --keyring.accname validator \
  --gateway.addr 0.0.0.0 \
  --rpc.addr 0.0.0.0 \
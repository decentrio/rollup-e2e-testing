# Register the validator EVM address
{
  # wait for block 1
  sleep 20

  # private key: da6ed55cb2894ac2c9c10209c09de8e8b9d109b910338d5bf3d747a7e1fc9eb9
  celestia-appd tx qgb register \
    "$(celestia-appd keys show validator --home $1 --bech val -a)" \
    0x966e6f22781EF6a6A82BBB4DB3df8E225DfD9488 \
    --from validator \
    --home $1 \
    --fees 30000utia -b block \
    --chain-id test \
    -y
} &

mkdir -p /home/celestia/light/keys
cp -r $1/keyring-test/ /home/celestia/light/keys/keyring-test/

# Start the celestia-app
celestia-appd start --home $1 &
sleep 1000000000
# Try to get the genesis hash. Usually first request returns an empty string (port is not open, curl fails), later attempts
# returns "null" if block was not yet produced.
# GENESIS=
# CNT=0
# MAX=30
# while [ "${#GENESIS}" -le 4 -a $CNT -ne $MAX ]; do
# 	GENESIS=$(curl -s http://127.0.0.1:26657/block?height=1 | jq '.result.block_id.hash' | tr -d '"')
# 	((CNT++))
# 	sleep 10
# done
# echo $GENESIS
# export CELESTIA_CUSTOM=test:$GENESIS
# echo "$CELESTIA_CUSTOM"

# celestia light init --node.store /home/celestia/light
celestia light start --node.store /home/celestia/light --gateway --core.ip public-celestia-mocha4-consensus.numia.xyz --keyring.accname validator --p2p.network mocha-4 --headers.trusted-hash 496BA2F12B9B64789DF8802FB75CB65161519F4FECC68774BCE0118FF2098322
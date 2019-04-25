# Bookkeeper
Bookkeeper = Dumper + Processor and handles bookkeeping.

Bookkeeper can handle up to 300 voters.

Attention:
This Bookkeeper is a REFERENCE IMPLEMENTATION of reward distribution tool provided by IOTEX FOUNDATION. IOTEX FOUNDATION disclaims all responsibility for any damages or losses (including, without limitation, financial loss, damages for loss in business projects, loss of profits or other consequential losses) arising in contract, tort or otherwise from the use of or inability to use the Bookkeeper, or from any action or decision taken as a result of using this Bookkeeper.


## Build
```
# install dependencies
dep ensure

# build the project
make build
```

## Get Voters' Rewards by Delegate Name
Usage: `bookkeeper --bp BP_NAME --start START_EPOCH_NUM --to END_EPOCH_NUM --percentage PERCENTAGE [--with-foundation-bonus] [--endpoint IOTEX_ENDPOINT] [--CONFIG CONFIG_FILE]`

For example, delegate `iotexlab` wants to distribute 90% of its reward from epoch 24 to epoch 48. If iotexlab only wants to distribute Epoch Reward:

```
./bookkeeper --bp iotexlab --start 24 --to 48 --percentage 90
```

If iotexlab also wants to distribute Foundation Bonus in addition to Epoch Reward:

```
./bookkeeper --bp iotexlab --start 24 --to 48 --percentage 90 --with-foundation-bonus
```

The result will be saved to file `epoch_24_to_48.csv`, with the first column as the voter address, and the second column as the reward in Rau the corresponding voter will get.

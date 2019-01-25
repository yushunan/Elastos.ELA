package version

import (
	"os"

	"github.com/elastos/Elastos.ELA/blockchain/interfaces"
	"github.com/elastos/Elastos.ELA/dpos/log"
	"github.com/elastos/Elastos.ELA/version/blocks"
	"github.com/elastos/Elastos.ELA/version/heights"
	"github.com/elastos/Elastos.ELA/version/txs"
	"github.com/elastos/Elastos.ELA/version/verconf"
)

const (
	versionCount = 4
)

func NewVersions(cfg *verconf.Config) interfaces.HeightVersions {
	if len(cfg.ChainParams.HeightVersions) < versionCount {
		log.Fatal("insufficient height version count")
		os.Exit(1)
	}

	txV0 := txs.NewTxV0(cfg)
	txV1 := txs.NewTxV1(cfg)
	txCurrent := txs.NewTxV2(cfg)

	blockV0 := blocks.NewBlockV0(cfg)
	blockV1 := blocks.NewBlockV1(cfg)
	blockCurrent := blocks.NewBlockV2(cfg)

	versions := heights.NewHeightVersions(
		map[uint32]heights.VersionInfo{
			cfg.ChainParams.HeightVersions[0]: {
				0,
				0,
				map[byte]txs.TxVersion{txV0.GetVersion(): txV0},
				map[uint32]blocks.BlockVersion{blockV0.GetVersion(): blockV0},
			},
			cfg.ChainParams.HeightVersions[1]: {
				1,
				0,
				map[byte]txs.TxVersion{txV1.GetVersion(): txV1},
				map[uint32]blocks.BlockVersion{blockV0.GetVersion(): blockV0},
			},
			cfg.ChainParams.HeightVersions[2]: {
				9,
				1,
				map[byte]txs.TxVersion{txCurrent.GetVersion(): txCurrent},
				map[uint32]blocks.BlockVersion{blockV1.GetVersion(): blockV1},
			},
			cfg.ChainParams.HeightVersions[3]: {
				9,
				2,
				map[byte]txs.TxVersion{txCurrent.GetVersion(): txCurrent},
				map[uint32]blocks.BlockVersion{blockCurrent.GetVersion(): blockCurrent},
			},
		},
		cfg.ChainParams.HeightVersions[2],
	)
	cfg.Versions = versions
	return versions
}
package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elastos/Elastos.ELA/blockchain"
	clicom "github.com/elastos/Elastos.ELA/cli/common"
	"github.com/elastos/Elastos.ELA/common/log"
	"github.com/elastos/Elastos.ELA/core/types"
	log2 "github.com/elastos/Elastos.ELA/dpos/log"
	"github.com/elastos/Elastos.ELA/servers"
	"github.com/elastos/Elastos.ELA/version/verconfig"

	"github.com/elastos/Elastos.ELA.Utility/http/jsonrpc"
	"github.com/elastos/Elastos.ELA.Utility/http/util"
	"github.com/elastos/Elastos.ELA/common"
	"github.com/yuin/gopher-lua"
)

func Loader(L *lua.LState) int {
	// register functions to the table
	mod := L.SetFuncs(L.NewTable(), exports)
	// register other stuff
	L.SetField(mod, "version", lua.LString("0.1"))

	// returns the module
	L.Push(mod)
	return 1
}

var exports = map[string]lua.LGFunction{
	"hex_reverse":       hexReverse,
	"send_tx":           sendTx,
	"get_asset_id":      getAssetID,
	"get_utxo":          getUTXO,
	"init_ledger":       initLedger,
	"close_store":       closeStore,
	"clear_store":       clearStore,
	"get_dir_all_files": getDirAllFiles,
}

func getDirAllFiles(L *lua.LState) int {
	str := L.ToString(1)

	files, err := walkDir(str, ".lua")
	if err != nil {
		fmt.Println("Read directory error: " + err.Error())
	}

	table := L.NewTable()
	L.SetMetatable(table, L.GetTypeMetatable(luaStringsTypeName))
	for _, f := range files {
		table.Append(lua.LString(f))
	}
	L.Push(table)

	return 1
}

func hexReverse(L *lua.LState) int {
	str := L.ToString(1)
	buf, _ := hex.DecodeString(str)
	retHex := hex.EncodeToString(common.BytesReverse(buf))

	L.Push(lua.LString(retHex))
	return 1
}

func sendTx(L *lua.LState) int {
	txn := checkTransaction(L, 1)

	var buffer bytes.Buffer
	err := txn.Serialize(&buffer)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	txHex := hex.EncodeToString(buffer.Bytes())

	result, err := jsonrpc.CallParams(clicom.LocalServer(), "sendrawtransaction", util.Params{
		"data": txHex,
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	L.Push(lua.LString(result.(string)))

	return 1
}

func getAssetID(L *lua.LState) int {
	L.Push(lua.LString("a3d0eaa466df74983b5d7c543de6904f4c9418ead5ffd6d25814234a96db37b0"))
	return 1
}

func getUTXO(L *lua.LState) int {
	from := L.ToString(1)
	result, err := jsonrpc.CallParams(clicom.LocalServer(), "listunspent", util.Params{
		"addresses": []string{from},
	})
	if err != nil {
		return 0
	}
	data, err := json.Marshal(result)
	if err != nil {
		return 0
	}
	var utxos []servers.UTXOInfo
	err = json.Unmarshal(data, &utxos)

	var availabelUtxos []servers.UTXOInfo
	for _, utxo := range utxos {
		if types.TransactionType(utxo.TxType) == types.CoinBase && utxo.Confirmations < 100 {
			continue
		}
		availabelUtxos = append(availabelUtxos, utxo)
	}

	ud := L.NewUserData()
	ud.Value = availabelUtxos
	L.SetMetatable(ud, L.GetTypeMetatable(luaClientTypeName))
	L.Push(ud)

	return 1
}

func initLedger(L *lua.LState) int {
	logLevel := uint8(L.ToInt(1))
	arbitrators := checkArbitrators(L, 2)

	log.Init(logLevel, 0, 0)
	log2.Init(logLevel, 0, 0)

	versions := verconfig.InitVersions()
	chainStore, err := blockchain.NewChainStore("Chain_WhiteBox")
	if err != nil {
		fmt.Printf("Init chain store error: %s \n", err.Error())
	}

	err = blockchain.Init(chainStore, versions)
	if err != nil {
		fmt.Printf("Init block chain error: %s \n", err.Error())
	}

	blockchain.DefaultLedger.Arbitrators = arbitrators

	return 1
}

func closeStore(L *lua.LState) int {
	blockchain.DefaultLedger.Store.Close()

	return 0
}

func clearStore(L *lua.LState) int {
	os.RemoveAll("Chain_WhiteBox/")
	return 0
}

func walkDir(dirPth, suffix string) (files []string, err error) {
	files = make([]string, 0, 30)
	suffix = strings.ToUpper(suffix)
	err = filepath.Walk(dirPth, func(filename string, fi os.FileInfo, err error) error {
		if fi.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToUpper(fi.Name()), suffix) {
			files = append(files, filename)
		}
		return nil
	})
	return files, err
}

func RegisterDataType(L *lua.LState) int {
	RegisterClientType(L)
	RegisterAttributeType(L)
	RegisterInputType(L)
	RegisterOutputType(L)
	RegisterDefaultOutputType(L)
	RegisterVoteOutputType(L)
	RegisterVoteContentType(L)
	RegisterTransactionType(L)
	RegisterCoinBaseType(L)
	RegisterTransferAssetType(L)
	RegisterTransactionType(L)
	RegisterReturnDepositCoinType(L)
	RegisterProposalType(L)
	RegisterVoteType(L)
	RegisterConfirmType(L)
	RegisterBlockType(L)
	RegisterHeaderType(L)
	RegisterDposNetworkType(L)
	RegisterDposManagerType(L)
	RegisterArbitratorsType(L)
	RegisterRegisterProducerType(L)
	RegisterUpdateProducerType(L)
	RegisterCancelProducerType(L)
	RegisterIllegalProposalsType(L)
	RegisterIllegalVotesType(L)
	RegisterIllegalBlocksType(L)
	RegisterStringsType(L)

	return 0
}

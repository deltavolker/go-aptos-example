package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"go-aptos-example/base"
	"math/big"
	"strconv"

	"github.com/coming-chat/go-aptos/aptosclient"
	"github.com/coming-chat/go-aptos/aptostypes"
	transactionbuilder "github.com/coming-chat/go-aptos/transaction_builder"
	"github.com/coming-chat/lcs"
	"github.com/coming-chat/wallet-SDK/core/aptos"
	"github.com/omnibtc/go-aptos-liquidswap/liquidswap"
	"github.com/shopspring/decimal"
)

const (
	swapAbiFormat = "010473776170%s0773637269707473a501205377617020657861637420636f696e2060586020666f72206174206c65617374206d696e696d756d20636f696e206059602e0a202a2060636f696e5f76616c60202d20616d6f756e74206f6620636f696e732060586020746f20737761702e0a202a2060636f696e5f6f75745f6d696e5f76616c60202d206d696e696d756d20657870656374656420616d6f756e74206f6620636f696e732060596020746f206765742e03017801790563757276650208636f696e5f76616c0210636f696e5f6f75745f6d696e5f76616c02"
	// swapIntoAbi
	// 0109737761705f696e746f%s0773637269707473ab012053776170206d6178696d756d20636f696e2060586020666f7220657861637420636f696e206059602e0a202a2060636f696e5f76616c5f6d617860202d20686f77206d756368206f6620636f696e73206058602063616e206265207573656420746f206765742060596020636f696e2e0a202a2060636f696e5f6f757460202d20686f77206d756368206f6620636f696e73206059602073686f756c642062652072657475726e65642e0301780179056375727665020c636f696e5f76616c5f6d61780208636f696e5f6f757402

	APTOS = "0x1::aptos_coin::AptosCoin"
	USDT  = "0x43417434fd869edee76cca2a4d2301e528a1551b1d719b75c350c3c97d15b8b9::coins::USDT"
	BTC   = "0x43417434fd869edee76cca2a4d2301e528a1551b1d719b75c350c3c97d15b8b9::coins::BTC"
	Pool  = ""

	scriptAddress = "0x4e9fce03284c0ce0b86c88dd5a46f050cad2f4f33c4cdd29d98f501868558c81" // 0x43417434fd869edee76cca2a4d2301e528a1551b1d719b75c350c3c97d15b8b9::liquidity_pool::LiquidityPool<CoinA, CoinB, Pool>
	poolAddress   = "0x8aa500cd155a6087509fa84bc7f0deed3363dd253ecb62b2f110885dacf01c67" // 0x43417434fd869edee76cca2a4d2301e528a1551b1d719b75c350c3c97d15b8b9::lp::LP<CoinA, CoinB>
)
const upperhex = "0123456789ABCDEF"

var (
	address2Coin map[string]liquidswap.Coin
)

func init() {
	address2Coin = make(map[string]liquidswap.Coin)
	address2Coin[APTOS] = liquidswap.Coin{
		Decimals: 8,
		Symbol:   "APTOS",
	}
	address2Coin[USDT] = liquidswap.Coin{
		Decimals: 6,
		Symbol:   "USDT",
	}
	address2Coin[BTC] = liquidswap.Coin{
		Decimals: 8,
		Symbol:   "BTC",
	}

	lcs.RegisterEnum(
		(*transactionbuilder.TypeTag)(nil),

		transactionbuilder.TypeTagBool{},
		transactionbuilder.TypeTagU8{},
		transactionbuilder.TypeTagU64{},
		transactionbuilder.TypeTagU128{},
		transactionbuilder.TypeTagAddress{},
		transactionbuilder.TypeTagSigner{},
		transactionbuilder.TypeTagVector{},
		transactionbuilder.TypeTagStruct{},
	)
}

func main() {
	account, err := base.GetEnvAptosAccount()
	base.PanicError(err)

	chain := base.GetChain()

	// 构造交易，预估得到的 coin，执行 swap，查看交易详情
	swap(account, chain, APTOS, USDT, "100000")
}

func swap(account *aptos.Account, chain *aptos.Chain, fromCoinAddress, toCoinAddress, fromAmount string) {
	// 获取 resource
	client, err := chain.GetClient()
	base.PanicError(err)
	fromCoin := address2Coin[fromCoinAddress]
	toCoin := address2Coin[toCoinAddress]
	xAddress, yAddress := fromCoinAddress, toCoinAddress
	if !liquidswap.IsSortedSymbols(fromCoin.Symbol, toCoin.Symbol) {
		xAddress, yAddress = yAddress, xAddress
	}
	p := getPoolReserve(client, scriptAddress, poolAddress, xAddress, yAddress, fmt.Sprintf("%s::curves::Uncorrelated", scriptAddress))
	amount, b := big.NewInt(0).SetString(fromAmount, 10)
	if !b {
		panic("invali params")
	}
	fmt.Printf("x: %s, %s\ny: %s %s\n", p.CoinXReserve, xAddress, p.CoinYReserve, yAddress)

	res := liquidswap.GetAmountOut(fromCoin, toCoin, amount, p)
	fmt.Printf("in %s: %s, out %s: %s\n", fromCoin.Symbol, amount.String(), toCoin.Name, res.String())

	payload, err := liquidswap.CreateSwapPayload(&liquidswap.SwapParams{
		Script:           scriptAddress + "::scripts",
		FromCoin:         fromCoinAddress,
		ToCoin:           toCoinAddress,
		FromAmount:       amount,
		ToAmount:         res,
		InteractiveToken: "from",
		Slippage:         decimal.NewFromFloat(0.005),
		Pool: liquidswap.Pool{
			CurveStructType: fmt.Sprintf("%s::curves::Uncorrelated", scriptAddress),
		},
	})
	base.PanicError(err)

	fmt.Printf("%v", payload)

	// abi
	abiStr := fmt.Sprintf(swapAbiFormat, scriptAddress[2:])
	swapbytes, err := hex.DecodeString(abiStr)
	base.PanicError(err)
	abiBytes := [][]byte{
		swapbytes,
	}
	abi, err := transactionbuilder.NewTransactionBuilderABI(abiBytes)
	base.PanicError(err)

	// encode args
	arg1, err := strconv.ParseUint(payload.Args[0], 10, 64)
	base.PanicError(err)
	arg2, err := strconv.ParseUint(payload.Args[1], 10, 64)
	base.PanicError(err)
	args := []interface{}{
		arg1,
		arg2,
	}

	payloadBcs, err := abi.BuildTransactionPayload(payload.Function, payload.TypeArgs, args)
	base.PanicError(err)
	bcsBytes, err := lcs.Marshal(payloadBcs)
	base.PanicError(err)

	ensureRegisterCoin(account, chain, toCoinAddress)

	hash, err := chain.SubmitTransactionPayloadBCS(account, bcsBytes)
	base.PanicError(err)
	println(hash)
}

func ensureRegisterCoin(account *aptos.Account, chain *aptos.Chain, toCoinAddress string) {
	token, err := aptos.NewToken(chain, toCoinAddress)
	base.PanicError(err)
	_, err = token.EnsureOwnerRegistedToken(account)
	base.PanicError(err)
}

func getPoolReserve(client *aptosclient.RestClient, scriptAddress, poolAddress, xAddress, yAddress string, curveType string) liquidswap.PoolResource {
	poolResourceType := fmt.Sprintf(
		"%s::liquidity_pool::LiquidityPool<%s,%s,%s>",
		scriptAddress,
		xAddress, // 这两个顺序与 lp 不一致
		yAddress,
		curveType,
	)
	// /0x43417434fd869edee76cca2a4d2301e528a1551b1d719b75c350c3c97d15b8b9::liquidity_pool::LiquidityPool
	// <0x1::aptos_coin::AptosCoin,
	// 0x43417434fd869edee76cca2a4d2301e528a1551b1d719b75c350c3c97d15b8b9::coins::USDT,
	// 0x43417434fd869edee76cca2a4d2301e528a1551b1d719b75c350c3c97d15b8b9::lp::LP
	// <0x1::aptos_coin::AptosCoin,
	// %200x43417434fd869edee76cca2a4d2301e528a1551b1d719b75c350c3c97d15b8b9::coins::USDT>>
	// poolResourceType = escapeTypes(poolResourceType)
	resource, err := client.GetAccountResource(poolAddress, poolResourceType, 0)
	if err != nil {
		base.PanicError(err)
	}

	return resourceToPoolReserve(resource, true)
}

func resourceToPoolReserve(resource *aptostypes.AccountResource, reverse bool) liquidswap.PoolResource {
	x := resource.Data["coin_x_reserve"].(map[string]interface{})["value"].(string)
	y := resource.Data["coin_y_reserve"].(map[string]interface{})["value"].(string)
	xint, b := big.NewInt(0).SetString(x, 10)
	if !b {
		base.PanicError(errors.New("invalid reserve"))
	}
	yint, b := big.NewInt(0).SetString(y, 10)
	if !b {
		base.PanicError(errors.New("invalid reserve"))
	}
	s := liquidswap.PoolResource{
		CoinXReserve: xint,
		CoinYReserve: yint,
	}
	if reverse {
		s.CoinXReserve, s.CoinYReserve = s.CoinYReserve, s.CoinXReserve
	}
	return s
}

func escapeTypes(s string) string {
	spaceCount, hexCount := 0, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscape(c) {
			if c == ' ' {
				spaceCount++
			} else {
				hexCount++
			}
		}
	}

	if spaceCount == 0 && hexCount == 0 {
		return s
	}

	var buf [64]byte
	var t []byte
	required := len(s) + 2*hexCount
	if required <= len(buf) {
		t = buf[:required]
	} else {
		t = make([]byte, required)
	}
	if hexCount == 0 {
		copy(t, s)
		for i := 0; i < len(s); i++ {
			if s[i] == ' ' {
				t[i] = '+'
			}
		}
		return string(t)
	}
	j := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == ' ':
			t[j] = '+'
			j++
		case shouldEscape(c):
			t[j] = '%'
			t[j+1] = upperhex[c>>4]
			t[j+2] = upperhex[c&15]
			j += 3
		default:
			t[j] = s[i]
			j++
		}
	}
	return string(t)
}

func shouldEscape(c byte) bool {
	return c == '<' || c == '>'
}

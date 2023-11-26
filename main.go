package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"io/ioutil"
	"log"
	"math/big"
	"math/rand"
	"strings"
	"sync"
	"time"
)

type Wallet struct {
	Private string `json:"private"`
	Address string `json:"address"`
}

type Token struct {
	Tick  string `json:"tick"`
	Amt   string `json:"amt"`
	Workc string `json:"workc"`
}

type Network struct {
	RpcURL string `json:"rpcURL"`
}

type Config struct {
	Wallets []Wallet `json:"wallets"`
	Tokens  []Token  `json:"tokens"`
	Network Network  `json:"network"`
}

var (
	transactionSent = false
	mu              sync.Mutex
)

var (
	token   = flag.String("token", "", "example: ierc-m4")
	address = flag.String("address", "", "example: 0x000000")
	count   = flag.Int("count", 1, "单账号挖矿次数，默认为1")
	thread  = flag.Int("thread", 1000, "并发线程数，默认为1000")
	gas     = flag.Int("gas", 0, "gas价格，默认为0，自动获取")
	test    = flag.Bool("test", false, "是否为测试模式，默认为false。测试模式只会计算tx、不发送真实的交易")
)

func main() {
	//打印开发者信息
	fmt.Println("作者: @longtao_eth  https://twitter.com/longtao_eth")
	fmt.Println("Author: @longtao_eth  https://twitter.com/longtao_eth")
	flag.Parse()
	//读取config.json配置文件
	configFile, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("读取config.json配置文件失败")
		return
	}
	//解析config.json配置文件
	var config Config
	err = json.Unmarshal(configFile, &config)
	if err != nil {
		fmt.Println("解析config.json配置文件失败")
		return
	}
	//遍历config.json配置文件，获取对应的私钥、地址、token
	for _, wallet := range config.Wallets {
		if wallet.Address == *address {
			for _, mintToken := range config.Tokens {
				if mintToken.Tick == *token {
					for i := 0; i < *count; i++ {
						transactionSent = false
						rand.Seed(time.Now().UnixNano())
						pow(wallet, mintToken, config.Network, *thread, *gas, *test)
					}
				}
			}
		}
	}

}

func pow(wallet Wallet, token Token, network Network, thread int, gas int, test bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // cancel when we are finished
	//连接以太坊客户端
	client, err := ethclient.Dial(network.RpcURL)
	if err != nil {
		return
	}

	//将私钥转换为ECDSA
	privateKeyECDSA, err := crypto.HexToECDSA(wallet.Private)
	if err != nil {
		return
	}

	//获取公钥
	publicKey := privateKeyECDSA.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		panic("error casting public key to ECDSA")
		return
	}

	//获取地址
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	//获取待处理的交易数
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return
	}
	var gasPrice *big.Int
	if gas == 0 {
		//获取建议的gas价格
		gasPrice, err = client.SuggestGasPrice(context.Background())
		if err != nil {
			return
		}
	} else {
		gasPrice = big.NewInt(int64(gas) * 1e9)
	}

	//创建交易签名者
	auth := types.NewEIP155Signer(big.NewInt(1)) // 1 is the chain ID for the Ethereum mainnet
	value := big.NewInt(0)                       // in wei (1 eth = 10^18 wei)

	toAddress := common.HexToAddress("0x0000000000000000000000000000000000000000")
	//统计单个计算tx所耗费时间
	start := time.Now()
	var TrueHash string
	//创建x个线程并发计算txhash，若计算成果则发送交易并关闭所有线程
	var wg sync.WaitGroup
	for i := 0; i < thread; i++ {
		wg.Add(1)
		go func(nonce uint64) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				mu.Lock()
				if transactionSent {
					mu.Unlock()
					return
				}
				mu.Unlock()
				randomDigits := generateRandomDigits(5)
				currentTime := time.Now().Unix()
				timer := fmt.Sprintf("%d%s", currentTime, randomDigits)
				dataString := fmt.Sprintf(`data:application/json,{"p":"ierc-20","op":"mint","tick":"%s","amt":"%s","nonce":"%s"}`, token.Tick, token.Amt, timer)
				//dataHex := hex.EncodeToString([]byte(dataString))
				tx := types.LegacyTx{
					Nonce:    nonce,
					GasPrice: gasPrice,
					Gas:      28000,
					To:       &toAddress,
					Value:    value,
					Data:     []byte(dataString),
				}
				transaction := types.NewTx(&tx)
				signedTx, err := types.SignTx(transaction, auth, privateKeyECDSA)
				if err != nil {
					log.Fatalf("Failed to sign transaction: %v", err)
				}
				hash := signedTx.Hash().Hex()
				fmt.Printf("\rFalseHash: %s\n", hash)
				if strings.Contains(hash, token.Workc) {
					TrueHash = hash
					mu.Lock()
					transactionSent = true
					mu.Unlock()
					//发送交易
					if test == false {
						err = client.SendTransaction(context.Background(), signedTx)
						if err != nil {
							log.Fatalf("Failed to send transaction: %v", err)
						}
					}
					cancel()
					break
				}
			}
		}(nonce)
	}
	wg.Wait()
	fmt.Printf("TrueHash: %s\n", TrueHash)
	stop := time.Now()
	fmt.Printf("单个txhash所耗费时间: %v\n", stop.Sub(start))
}

func generateRandomDigits(length int) string {
	digits := make([]byte, length)
	for i := range digits {
		digits[i] = '0' + byte(rand.Intn(10))
	}
	return string(digits)
}

package main

import (
	"bytes"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	fastHttp "github.com/valyala/fasthttp"
	proxy "github.com/yeqown/fasthttp-reverse-proxy"
	"log"
	"math/rand"
	"strings"
	"sync"
	pb_tencent "tencentgo/model/tencent"
	"time"
)

type RConfig struct {
	UpstreamAddrs       []string
	DefaultUpstreamAddr string
	ListenPort          string
	TimesBackToSource   int
	NoExt               int
}

type upStreamStruct struct {
	ipAddr string
	deals  []string
}

type handle struct {
	addrs []string
}

type bodyContent struct {
	body string
	cnt  int
}

var (
	bodyMap = &sync.Map{}

	configMap map[string]upStreamStruct

	allDealsMap map[string]bool //存放所有deals
	rconfig     RConfig

	client = NewFastHttpClient()
)

func NewFastHttpClient() *fastHttp.Client {
	return &fastHttp.Client{
		MaxConnsPerHost:    1000,
		MaxConnWaitTimeout: 30 * time.Second,
	}
}

func FastHttpRoutrip(ctx *fastHttp.RequestCtx) *fastHttp.RequestCtx {

	req := &ctx.Request
	resp := &ctx.Response

	//process request change
	b := bytes.Replace(req.Body(), []byte("server"), []byte("schmerver"), -1)
	newRequest := &pb_tencent.Request{}
	err := proto.Unmarshal(b, newRequest)

	if err != nil {
		return nil
	}

	body, err := proto.Marshal(newRequest)
	if err != nil {
		log.Println(err)
		return nil
	}

	req.SetBody(body)

	if err := client.Do(req, resp); err != nil {
		log.Println("fasthttp do err is :", err)
		return nil
	}

	//response handler
	b = bytes.Replace(resp.Body(), []byte("server"), []byte("schmerver"), -1)
	newResponse := &pb_tencent.Response{}

	err = proto.Unmarshal(b, newResponse)
	dealid := newRequest.Impression[0].GetDealid()
	if len(newResponse.GetSeatbid()) > 0 && len(newResponse.GetSeatbid()[0].GetBid()) > 0 {
		adid := newResponse.Seatbid[0].Bid[0].GetAdid()
		bodyMap.Store(dealid, bodyContent{adid, 0})
	} else {
		bodyMap.Store(dealid, bodyContent{"0", 1})
	}

	//fmt.Println("roundTrip")
	fmt.Println("roundTrip REQREQREQREQ      " + newRequest.String())
	fmt.Println("roundTrip RESPRESPRESPRESP  " + newResponse.String())

	// pb object to response body and return to hhtp
	data, err := proto.Marshal(newResponse) //TODO: if no changed, just send original pb to http
	resp.SetBody(data)
	resp.Header.SetContentLength(len(data))
	return ctx
}

func (this *handle) ServeHTTP(ctx *fastHttp.RequestCtx) {
	b := ctx.Request.Body()

	//process request change
	b = bytes.Replace(b, []byte("server"), []byte("schmerver"), -1)
	newRequest := &pb_tencent.Request{}
	err := proto.Unmarshal(b, newRequest)

	if err != nil {
		log.Println("proto parse err is :", err)
		return
	}

	addr := rconfig.DefaultUpstreamAddr

	var newRequestDealId string
	if len(newRequest.Impression) > 0 {
		newRequestDealId = newRequest.Impression[0].GetDealid()

		if configMap == nil {
			panic("config map is nil")
		}

		for _, conV := range configMap {
			for _, vDs := range conV.deals {
				if strings.Contains(vDs, newRequestDealId) || strings.Contains(newRequestDealId, vDs) {
					addr = conV.ipAddr
					break
				}
			}
		}

		//remote, err := url.Parse("http://" + addr)
		if err != nil {
			panic(err)
		}
		//mutex.Lock()
		bodycontent, ok := bodyMap.Load(newRequestDealId)
		//mutex.Unlock()

		_, dealOk := allDealsMap[newRequestDealId] //判断此dealId 是否在配置文件deal列表中
		if ok && dealOk && bodycontent != nil {
			if rand.Intn(rconfig.TimesBackToSource) > 1 {
				fmt.Println(newRequestDealId + " ==>" + addr)
				id := newRequest.GetId()
				bidid := newRequest.Impression[0].GetId()
				adid := bodycontent.(bodyContent).body
				price := float32(9000)
				extid := "ssp" + adid
				//mutex.Lock()
				//bodyMap[dealid] = bodyContent{adid, bodycontent.cnt + 1}
				//mutex.Unlock()
				err = proto.Unmarshal(b, newRequest)
				newResponse := &pb_tencent.Response{}
				if adid != "0" {
					newResponse = &pb_tencent.Response{
						Id: &id,
						Seatbid: []*pb_tencent.Response_SeatBid{
							{
								Bid: []*pb_tencent.Response_Bid{
									{
										Id:    &bidid,
										Impid: &bidid,
										Price: &price,
										Adid:  &adid,
										Ext:   &extid},
								},
							},
						},
					}
				} else {
					newResponse = &pb_tencent.Response{
						Id: &id,
					}
				}
				data, err := proto.Marshal(newResponse) //TODO: if no changed, just send original pb to http
				if err != nil {
					ctx.Response.SetStatusCode(204)
					//w.WriteHeader(204)
				}
				ctx.Write(data)

				fmt.Println("serverHttp REQREQREQREQ         " + newRequest.String())
				fmt.Println("serverHttp RESPRESPRESPRESP     " + newResponse.String())
				return
			}
		}

		ctx.SetBody(b)
		ctx.Request.SetRequestURI("http://" + addr + "/tencent.htm")
		ctx.Request.Header.Set("Content-Type", "application/x-protobuf;charset=UTF-8")
		proxyServer := proxy.NewReverseProxy(addr)
		proxyServer.ServeHTTP(FastHttpRoutrip(ctx))
	}

}

func startServer() {
	//被代理的服务器host和port
	h := &handle{}

	err := fastHttp.ListenAndServe(":"+rconfig.ListenPort, h.ServeHTTP)

	if err != nil {
		log.Fatalln("start http err is : ", err)
	}
}

func main() {
	proxy.SetProduction()
	configMap = make(map[string]upStreamStruct)
	allDealsMap = make(map[string]bool)

	viper.SetConfigName("tencentconfig")
	viper.AddConfigPath(".")
	//bodyMap = make(map[string]bodyContent)
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	err = viper.Unmarshal(&rconfig)
	if err != nil {
		panic(err)
	}

	if len(rconfig.UpstreamAddrs) != 0 {
		for _, v := range rconfig.UpstreamAddrs {

			upStream := strings.Split(v, "|")
			//golang map 遍历输出无序的，所以加入id
			id := upStream[0]
			usSplit := strings.Split(upStream[1], ",")
			deals := usSplit[1:]
			uss := &upStreamStruct{
				ipAddr: usSplit[0],
				deals:  deals,
			}

			configMap[id] = *uss

			for _, cV := range configMap {
				for _, v := range cV.deals {
					allDealsMap[v] = true
				}
			}
		}
	}

	fmt.Println(rconfig)
	fmt.Println(allDealsMap)

	startServer()
}

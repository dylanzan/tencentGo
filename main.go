package main

import (
	"bytes"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	fastHttp "github.com/valyala/fasthttp"
	"log"
	"strings"
	"sync"
	"tencentgo/model"
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

	allDealsMap   map[string]bool //存放所有deals
	rconfig       RConfig
	client        = NewFastHttpClient()
	err           error
	upStreamCount int = 0
)

func NewFastHttpClient() *fastHttp.Client {
	return &fastHttp.Client{
		MaxConnsPerHost:    512000,
		MaxConnWaitTimeout: 10 * time.Second,
		//ReadTimeout:         8 * time.Second,
		//WriteTimeout:        8 * time.Second,
		//MaxIdleConnDuration: 8 * time.Second,
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

	//client := &fastHttp.Client{}
	if err := client.DoTimeout(req, resp, 5*time.Second); err != nil {
		log.Println("fasthttp do err is :", err)
		resp.SetStatusCode(204)
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

		if err != nil {
			panic(err)
		}

		bodycontent, ok := bodyMap.Load(newRequestDealId)

		_, dealOk := allDealsMap[newRequestDealId] //判断此dealId 是否在配置文件deal列表中
		if ok && dealOk && bodycontent != nil {
			if upStreamCount <= rconfig.TimesBackToSource {
				//fmt.Println(newRequestDealId + " ==>" + addr)
				id := newRequest.GetId()
				bidid := newRequest.Impression[0].GetId()
				adid := bodycontent.(bodyContent).body
				price := float32(9000)
				extid := "ssp" + adid

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
				}
				ctx.Write(data)

				fmt.Println("serverHttp REQREQREQREQ         " + newRequest.String())
				fmt.Println("serverHttp RESPRESPRESPRESP     " + newResponse.String())
				upStreamCount++
				return
			} else {
				upStreamCount = 0
			}
		}

		ctx.SetBody(b)
		ctx.Request.SetRequestURI("http://" + addr + "/tencent.htm")
		ctx.Request.Header.Set("Content-Type", "application/x-protobuf;charset=UTF-8")

		if fCtx := FastHttpRoutrip(ctx); fCtx == nil {
			log.Println("ProxyPoolHandler got an error: ", err)
			ctx.SetStatusCode(204)
			return
		}

	}
}

//收集数据
func DataReport(bidRequest *pb_tencent.Request) {

	adxc := new(model.AdxContext)
	device := bidRequest.GetDevice()

	if device != nil {
		if device.GetIdfa() != "" {
			adxc.AdxVisitorId = fmt.Sprintf("DEVICE_%v", device.GetIdfa())
			adxc.DeviceType = model.IDFA
			adxc.UserAgent = device.GetUa()
			adxc.BidTime = ""

		} else if device.GetImei() != "" {
			adxc.AdxVisitorId = fmt.Sprintf("DEVICE_%v", device.GetImei())

		} else if device.GetOpenudid() != "" {
			adxc.AdxVisitorId = fmt.Sprintf("DEVICE_%v", device.GetOpenudid())

		} else if device.GetAndroidid() != "" {
			adxc.AdxVisitorId = fmt.Sprintf("DEVICE_%v", device.GetAndroidid())

		} else if device.GetMac() != "" {
			adxc.AdxVisitorId = fmt.Sprintf("DEVICE_%v", device.GetMac())

		} else {
			if bidRequest.GetUser() != nil {
				adxc.AdxVisitorId = fmt.Sprintf("DEVICE_%v", bidRequest.GetUser().GetId())
				//adxc.BidRequest.DeviceId = bidRequest.GetUser().GetId()
			}
		}
	} else {
		if bidRequest.GetUser() != nil {
			adxc.AdxVisitorId = bidRequest.GetUser().GetId()
			//adxc.BidRequest.DeviceId = bidRequest.GetUser().GetId()
		}
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

	//initProxy() //初始化代理池
	//proxy.SetProduction()
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

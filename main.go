package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	fastHttp "github.com/valyala/fasthttp"
	"log"
	"math/rand"
	"net/url"
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
	fasthttpClient =NewFastHttpClient
	reqIns =&Req{}
)

func NewFastHttpClient() *fastHttp.Client {
	return &fastHttp.Client{
		MaxConnsPerHost:    100,
		MaxConnWaitTimeout: 30 * time.Second,
	}
}


type Req struct {
	ctx context.Context
}

func (r Req)RoundTrip(ctx *fastHttp.RequestCtx) *fastHttp.RequestCtx{

	defer func() {
		err:=recover()

		if err!=nil{

		}
	}()

	req:=&ctx.Request
	resp:=&ctx.Response
	if r.ctx==nil || ctx.Request.Body()==nil{
		return nil
	}

	b:=ctx.Request.Body()

	//process request change
	bodyByte := bytes.Replace(b, []byte("server"), []byte("schmerver"), -1)
	newRequest:=&pb_tencent.Request{}

	err:=proto.Unmarshal(bodyByte,newRequest)

	if err!=nil{
		log.Println(err)
		resp.SetStatusCode(204)
		return nil
	}

	req.SetBody(bodyByte)

	if err:=fastHttp.Do(req,resp);err!=nil{
		resp.SetStatusCode(204)
		log.Println(err)
	}

	if resp==nil{
		return nil
	}

	newResponse:=&pb_tencent.Response{}

	respBody:=resp.Body()

	err=proto.Unmarshal(respBody,newResponse)

	if err!=nil{
		log.Println("proto parse err is :",err)
	}

	respBody=bytes.Replace(respBody, []byte("server"), []byte("schmerver"), -1)

	dealid := newRequest.Impression[0].GetDealid()
	if len(newResponse.GetSeatbid()) > 0 && len(newResponse.GetSeatbid()[0].GetBid()) > 0 {
		adid := newResponse.Seatbid[0].Bid[0].GetAdid()
		bodyMap.Store(dealid, bodyContent{adid, 0})
	} else {
		bodyMap.Store(dealid, bodyContent{"0", 1})
	}

	data,err:=proto.Marshal(newResponse)
	if err!=nil{
		log.Println("proto marshal err is ",err)
	}

	resp.SetBody(data)

	resp.Header.SetContentLength(len(data))

	return  ctx

}

func (this *handle) ServeHTTP(ctx *fastHttp.RequestCtx) {

	req:=&ctx.Request
	resp:=&ctx.Response

	b:=req.Body()

	//process request change
	b = bytes.Replace(b, []byte("server"), []byte("schmerver"), -1)
	newRequest := &pb_tencent.Request{}
	err := proto.Unmarshal(b, newRequest)
	//res, _ := json.Marshal(newRequest)

	if err!=nil{
		log.Println("parse newquest body err is ",err)
	}
	//log.Println(string(res))
	addr := rconfig.DefaultUpstreamAddr
	//if newRequest.Device.DeviceId != nil {

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

		remote, err := url.Parse("http://" + addr)
		if err != nil {
			panic(err)
		}
		// if  in bodyMap,return body directly
		//	dealid := newRequest.Impression[0].GetDealid()
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
					resp.SetStatusCode(204)
				}
				//bodyMap[*(newRequest.Impression[0].Dealid)] = bodyContent{bodycontent.body, bodycontent.cnt + 1}
				resp.SetBody(data)
				//fmt.Println("serverHttp")
				fmt.Println("serverHttp REQREQREQREQ         " + newRequest.String())
				fmt.Println("serverHttp RESPRESPRESPRESP     " + newResponse.String())
				return
			}
		}

		req.SetBody(b)
		req.Header.SetHost(remote.String())



		responseCtx:=reqIns.RoundTrip(ctx)

		resp.Header.SetContentLength(len(b))
		resp.SetBody(responseCtx.Response.Body())
		//if not in bodyMap, reverseProxy and transpot RoundTrip,
		//body := ioutil.NopCloser(bytes.NewReader(b))
		//r.Body = body
		//proxy := httputil.NewSingleHostReverseProxy(remote)
		//proxy.Transport = &transport{http.DefaultTransport}
		//proxy.ServeHTTP(w, r)

	}

}

func startServer() {
	//被代理的服务器host和port
	/*h := &handle{}

	srv := http.Server{
		Addr:    ":" + rconfig.ListenPort,
		Handler: h,
		//ReadTimeout:  20 * time.Second,
		//WriteTimeout: 20 * time.Second,
	}

	fmt.Println(srv.Addr)
	err := srv.ListenAndServe()

	if err != nil {
		log.Fatalln("ListenAndServe: ", err)
	}*/



}

func main() {

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

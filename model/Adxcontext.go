package model

const (
	IDFA      string = "IDFA"
	IMEI      string = "IMEI"
	OPENUUID  string = "OPENUUID"
	ANDROIDID string = "ANDROIDID"
	PC        string = "PC"
)

type AdxContext struct {
	AdxVisitorId string
	DeviceType   string
	IpAddress    string
	BidTime      string
	UserAgent    string
}

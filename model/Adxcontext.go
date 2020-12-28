package model

const (
	IDFA      string = "IDFA"
	IMEI      string = "IMEI"
	OPENUUID  string = "OPENUUID"
	ANDROIDID string = "ANDROIDID"
	PC        string = "PC"
	MAC       string = "MAC"
)

type AdxContext struct {
	AdxVisitorId string
	DeviceType   string
	IpAddress    string
	BidTime      string
	UserAgent    string
}

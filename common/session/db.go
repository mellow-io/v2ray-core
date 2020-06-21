package session

import (
	"sync"
	"time"
)

type DBService interface {
	InsertProxyLog(target, tag string, startTime, endTime int64, uploadBytes, downloadBytes int32, recordType, dnsQueryType int32, dnsRequest, dnsResponse string, dnsNumIPs int32)
}

var DefaultDBService DBService
var dbAccess sync.Mutex

func InsertRecord(record *ProxyRecord) {
	record.EndTime = time.Now().UnixNano()

	dbAccess.Lock()
	defer dbAccess.Unlock()

	if DefaultDBService != nil {
		DefaultDBService.InsertProxyLog(record.Target, record.Tag, record.StartTime, record.EndTime, record.UploadBytes, record.DownloadBytes, record.RecordType, record.DNSQueryType, record.DNSRequest, record.DNSResponse, record.DNSNumIPs)
	}
}

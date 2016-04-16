package stats

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
	"github.com/zenazn/goji/web"
)

type TimeSpan struct {
	Weeks				int
	Days				int
	Hours				int
	Minutes				int
	Seconds 			int
}

type ResponseURL struct {	
	ResponseURL 		string
	RepsonseMethod		string
	ResponseDuration	time.Duration
	ResponseTime		time.Time
	ResponseSeconds     float64
	ResponseSince       *TimeSpan
}

type Stats struct {
	mu                  sync.RWMutex
	Uptime              time.Time
	Pid                 int
	ResponseCounts      map[string]int
	TotalResponseCounts map[string]int
	URLRequestLatency   map[string]int
	URLRequestCounts    map[string]int
	TotalResponseTime   time.Time
	RequestTypeCounts	map[string]int
	UserAgentCounts		map[string]int
	URLHighestResponse  map[string]float64
	URLLowestResponse   map[string]float64
	MaxResponseTime		*ResponseURL
}

func New() *Stats {
	stats := &Stats{
		Uptime:              time.Now(),
		Pid:                 os.Getpid(),
		ResponseCounts:      map[string]int{},
		TotalResponseCounts: map[string]int{},
		URLRequestCounts:    map[string]int{},
		URLRequestLatency:   map[string]int{},
		TotalResponseTime:   time.Time{},
		RequestTypeCounts:	 map[string]int{},
		UserAgentCounts:	 map[string]int{},
		URLHighestResponse:  map[string]float64{},
		URLLowestResponse:   map[string]float64{},
		MaxResponseTime:     &ResponseURL{ ResponseURL: "", ResponseDuration: 0, ResponseTime: time.Time{}, ResponseSeconds: 0.0, ResponseSince: &TimeSpan{} },
	}

	go func() {
		for {
			stats.ResetResponseCounts()

			time.Sleep(time.Second * 1)
		}
	}()

	return stats
}

func (mw *Stats) ResetResponseCounts() {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.ResponseCounts = map[string]int{}
}

// MiddlewareFunc makes Stats implement the Middleware interface.
func (mw *Stats) Handler( c *web.C, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		beginning, recorder := mw.Begin(w)

		h.ServeHTTP(recorder, r)

		mw.End(beginning, recorder, r.URL.String(), r.Method, r.UserAgent() )
	})
}

// Negroni compatible interface
func (mw *Stats) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	beginning, recorder := mw.Begin(w)

	next(recorder, r)

	mw.End(beginning, recorder, r.URL.String(), r.Method, r.UserAgent() )
}

func (mw *Stats) Begin(w http.ResponseWriter) (time.Time, Recorder) {
	start := time.Now()

	writer := &RecorderResponseWriter{w, 200, 0}

	return start, writer
}

func (mw *Stats) EndWithStatus(start time.Time, status int, url string, method string, useragent string ) {
	end := time.Now()

	responseTime := end.Sub(start)

	mw.mu.Lock()

	defer mw.mu.Unlock()

	statusCode := fmt.Sprintf("%d", status)

	mw.ResponseCounts[statusCode]++
	mw.TotalResponseCounts[statusCode]++
	mw.TotalResponseTime = mw.TotalResponseTime.Add( responseTime )
	mw.URLRequestCounts[url]++
	mw.URLRequestLatency[url] += int( responseTime )
	mw.RequestTypeCounts[method]++
	mw.UserAgentCounts[useragent]++

	if mw.MaxResponseTime.ResponseDuration < responseTime {
		mw.MaxResponseTime.ResponseDuration = responseTime
		mw.MaxResponseTime.ResponseURL = url
		mw.MaxResponseTime.ResponseTime = time.Now()
		mw.MaxResponseTime.RepsonseMethod = method
	}

	if mw.URLHighestResponse[url] < float64( responseTime ) { mw.URLHighestResponse[url] = float64( responseTime )}
	if mw.URLLowestResponse[url] == float64( 0 ) { mw.URLLowestResponse[url] = float64( responseTime )}
	if float64( responseTime ) < mw.URLLowestResponse[url] { mw.URLLowestResponse[url] = float64( responseTime )}
}

func (mw *Stats) End(start time.Time, recorder Recorder, url string, method string, useragent string ) {
	mw.EndWithStatus(start, recorder.Status(), url, method, useragent )
}

type data struct {
	Pid                    int            `xml:"pid,attr" json:"pid"`
	UpTime                 string         `xml:"uptime,attr" json:"uptime"`
	UpTimeSec              float64        `xml:"uptime_sec,attr" json:"uptime_sec"`
	Time                   string         `xml:"time,attr" json:"time"`
	TimeUnix               int64          `xml:"unixtime,attr" json:"unixtime"`
	StatusCodeCount        map[string]int `xml:"status_code_count" json:"status_code_count"`
	TotalStatusCodeCount   map[string]int `xml:"total_status_code_count" json:"total_status_code_count"`
	Count                  int            `xml:"count,attr" json:"count"`
	TotalCount             int            `xml:"total_count,attr" json:"total_count"`
	TotalResponseTime      string         `xml:"total_response_time,attr" json:"total_response_time"`
	TotalResponseTimeSec   float64        `xml:"total_response_time_sec,attr" json:"total_response_time_sec"`
	AverageResponseTime    string         `xml:"average_response_time,attr" json:"average_response_time"`
	AverageResponseTimeSec float64        `xml:"average_response_time_sec,attr" json:"average_response_time_sec"`
	URLRequestCounts	   map[string]int `xml:"URLRequestCounts" json:"URLRequestCounts"`
	RequestTypeCounts	   map[string]int `xml:"RequestTypeCounts" json:"RequestTypeCounts"`
	UserAgentCounts	   	   map[string]int `xml:"UserAgentCounts" json:"UserAgentCounts"`
	URLRequestLatency      map[string]int `xml:"URLRequestLatency" json:"URLRequestLatency"`
	URLHighestResponse     map[string]float64 `xml:"URLHighestResponse" json:"URLHighestResponse"`
	URLLowestResponse      map[string]float64 `xml:"URLLowestResponse" json:"URLLowestResponse"`
	MaxResponseTime	   	   *ResponseURL   `xml:"MaxResponseTimes" json:"MaxResponseTimes"`
}

func (mw *Stats) Data() *data {

	mw.mu.RLock()

	now := time.Now()

	uptime := now.Sub(mw.Uptime)

	count := 0
	for _, current := range mw.ResponseCounts {
		count += current
	}

	totalCount := 0
	for _, count := range mw.TotalResponseCounts {
		totalCount += count
	}

	totalResponseTime := mw.TotalResponseTime.Sub(time.Time{})

	averageResponseTime := time.Duration(0)
	if totalCount > 0 {
		avgNs := int64(totalResponseTime) / int64(totalCount)
		averageResponseTime = time.Duration(avgNs)
	}

	mw.MaxResponseTime.ResponseSeconds = mw.MaxResponseTime.ResponseDuration.Seconds()

    duration := time.Since( mw.MaxResponseTime.ResponseTime )
    weeks, days, hours, minutes, seconds := SecondsToDate( Round( duration.Seconds(), 0 ))
	mw.MaxResponseTime.ResponseSince.Weeks = weeks
	mw.MaxResponseTime.ResponseSince.Days = days
	mw.MaxResponseTime.ResponseSince.Hours = hours
	mw.MaxResponseTime.ResponseSince.Minutes = minutes
	mw.MaxResponseTime.ResponseSince.Seconds = seconds


	r := &data{
		Pid:                    mw.Pid,
		UpTime:                 uptime.String(),
		UpTimeSec:              uptime.Seconds(),
		Time:                   now.String(),
		TimeUnix:               now.Unix(),
		StatusCodeCount:        mw.ResponseCounts,
		TotalStatusCodeCount:   mw.TotalResponseCounts,
		Count:                  count,
		TotalCount:             totalCount,
		TotalResponseTime:      totalResponseTime.String(),
		TotalResponseTimeSec:   totalResponseTime.Seconds(),
		AverageResponseTime:    averageResponseTime.String(),
		AverageResponseTimeSec: averageResponseTime.Seconds(),
		URLRequestCounts:       mw.URLRequestCounts,
		RequestTypeCounts:		mw.RequestTypeCounts,
		UserAgentCounts:		mw.UserAgentCounts,
		URLRequestLatency:      mw.URLRequestLatency,
		URLHighestResponse:     mw.URLHighestResponse,
		URLLowestResponse:      mw.URLLowestResponse,
		MaxResponseTime:		mw.MaxResponseTime,
	}

	mw.mu.RUnlock()

	return r
}

func SecondsToDate( seconds2 float64 ) ( int, int, int, int, int ) {
	seconds := int( seconds2 )
	weeks := seconds / (7*24*60*60)
	days := seconds / (24*60*60) - 7*weeks
	hours := seconds / (60*60) - 7*24*weeks - 24*days
	minutes := seconds / 60 - 7*24*60*weeks - 24*60*days - 60*hours
	seconds3 := seconds - 7*24*60*60*weeks - 24*60*60*days - 60*60*hours - 60*minutes

	return weeks, days, hours, minutes, seconds3
}

func Round(v float64, decimals int) float64 {
     var pow float64 = 1
     for i:=0; i<decimals; i++ {
         pow *= 10
     }
     return float64(int((v * pow) + 0.5)) / pow
}

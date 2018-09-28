package common

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const TimeFormat = "2006-01-02 15:04:05"
const TimeZoneFormat = "Z07"

type DateTime struct {
	Zone     int    `json:"timezone"`
	Datetime string `json:"datetime"`
	UTCTime  string `json:"utctime"`
	Country  string `json:"country"`
	City     string `json:"city"`
}

func FirstCharToUpper(str string) string {
	for i, v := range str {
		return string(unicode.ToUpper(v)) + str[i+1:]
	}
	return ""
}

func FirstCharToLower(str string) string {
	for i, v := range str {
		return string(unicode.ToLower(v)) + str[i+1:]
	}
	return ""
}

func CharsToString(ca []int8) string {
	s := make([]byte, len(ca))
	var lens int
	for ; lens < len(ca); lens++ {
		if ca[lens] == 0 {
			break
		}
		s[lens] = uint8(ca[lens])
	}
	return string(s[0:lens])
}

func ReadLines(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return []string{""}, err
	}
	defer f.Close()

	var ret []string

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			break
		}
		ret = append(ret, strings.Trim(line, "\n"))
	}

	return ret, nil
}

func GetDateTime() (localtime string, utctime string, zone int) {
	t := time.Now()
	var err error
	zone, err = strconv.Atoi(t.Format(TimeZoneFormat))
	if err != nil {
		zone = 0
		//logp.Warn("strconv.Atoi Err: ", err)
	}
	localtime = t.Format(TimeFormat)
	utctime = t.UTC().Format(TimeFormat)
	return
}

func GetLocation() (country, city string, err error) {
	// Redhat,CentOS
	// #cat /etc/sysconfig/clock
	// ZONE="Asia/Shanghai"
	path := "/etc/sysconfig/clock"
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	content := string(b)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		words := strings.Split(line, "=")
		if words[0] == "ZONE" {
			zone := strings.Trim(words[1], "\"")
			regAndCity := strings.Split(zone, "/")
			if len(regAndCity) != 2 {
				country = ""
				city = ""
				err = nil
				return
			}
			country = regAndCity[0]
			city = regAndCity[1]
			err = nil
			return
		}
	}

	// Ubuntu, Debian(releases Etch and later）
	// # cat /etc/timezone
	// America/New_York
	path = "/etc/timezone"
	b, err = ioutil.ReadFile(path)
	if err != nil {
		return
	}
	zone := string(b)
	regAndCity := strings.Split(zone, "/")
	if len(regAndCity) != 2 {
		country = ""
		city = ""
		err = nil
		return
	}
	country = regAndCity[0]
	city = regAndCity[1]
	err = nil
	return

	//Debian(releases Sarge and earlier)
	// /etc/localtime -> /usr/share/zoneinfo/Australia/Sydney
	// not suppport now
	return
}

func GetUTCTimestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

// DiffList : different list, return: left - right
// ex: left=[1,2,3], right=[2,3,4]. result=[1]
func DiffList(left []uint16, right []uint16) []uint16 {
	result := []uint16{}
	m := map[uint16]bool{}
	for _, e := range left {
		m[e] = true
	}
	for _, e := range right {
		m[e] = false
	}
	for e, v := range m {
		if v {
			result = append(result, e)
		}
	}
	return result
}

// AddList: left + right
// ex: left=[1,2,3], right=[2,3,4]. result=[1,2,3,2,3,4]
func AddList(left []uint16, right []uint16) []uint16 {
	return append(left, right...)
}

// CombineList: left + right, remove duplicate elements
// ex: left=[1,2,3], right=[2,3,4]. result=[1,2,3,4]
func CombineList(left []uint16, right []uint16) []uint16 {
	remain := DiffList(right, left)
	return AddList(left, remain)
}

// PrintStruct : print struct to json, for debug
func PrintStruct(data interface{}) {
	jsonbytes, err := json.Marshal(data)
	if err != nil {
		// log.Error("convert to json faild: ", err)
		return
	}
	fmt.Println(string(jsonbytes))
}

type ErrNotImplemented struct {
	OS string
}

func (e ErrNotImplemented) Error() string {
	return "not implemented on " + e.OS
}

// TryToInt converts value to int, if not a number, return the key
func TryToInt(key string) interface{} {
	value, err := strconv.ParseInt(key, 10, 64)
	if err != nil {
		return key
	}
	return value
}

// TryToNumber converts value to int, if not a number, return the key
func TryToNumber(key string) interface{} {
	intValue, err := strconv.ParseInt(key, 10, 64)
	if err == nil {
		return intValue
	}
	floatValue, err := strconv.ParseFloat(key, 64)
	if err == nil {
		return floatValue
	}
	return key
}

func TryToFloat64(number interface{}) (float64, bool) {
	rv := reflect.ValueOf(number)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(reflect.ValueOf(number).Int()), true
	case reflect.Uint, reflect.Uintptr, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(reflect.ValueOf(number).Uint()), true
	case reflect.Float32, reflect.Float64:
		return float64(reflect.ValueOf(number).Float()), true
	default:
		return 0, false
	}
}

// MakePifFilePath make a new pid path
// default pid lock file: procPath/pid.file
// or pid lock file: pidFilePath/procName.pid if set runPath
func MakePifFilePath(procName, runPath string) (string, error) {
	pidFilePath := ""
	if runPath == "" {
		absPath, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			return "", err
		}
		pidFilePath = filepath.Join(absPath, "pid.file")
	} else {
		// create pid lock file
		procName = procName + ".pid"
		pidFilePath = filepath.Join(runPath, procName)
	}
	return pidFilePath, nil
}

// reference from lockfile
func ScanPidLine(content []byte) int {
	if len(content) == 0 {
		return -1
	}

	var pid int
	if _, err := fmt.Sscanln(string(content), &pid); err != nil {
		return -1
	}

	if pid <= 0 {
		return -1
	}
	return pid
}

package core

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Sign implements the classic EPay MD5 protocol: sorted, decoded, non-empty values + key.
func Sign(v url.Values, key string) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		if k != "sign" && k != "sign_type" && v.Get(k) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+v.Get(k))
	}
	sum := md5.Sum([]byte(strings.Join(pairs, "&") + key))
	return hex.EncodeToString(sum[:])
}

func Verify(v url.Values, key string) bool {
	for _, values := range v {
		if len(values) != 1 {
			return false
		}
	}
	if t := v.Get("sign_type"); t != "" && !strings.EqualFold(t, "MD5") {
		return false
	}
	actual, err := hex.DecodeString(v.Get("sign"))
	expected, _ := hex.DecodeString(Sign(v, key))
	return err == nil && len(actual) == 16 && subtle.ConstantTimeCompare(actual, expected) == 1
}

var moneyPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})(\.[0-9]{1,2})?$`)

func ParseMoney(s string) (int64, error) {
	if !moneyPattern.MatchString(s) {
		return 0, errors.New("金额必须为最多两位小数的正数，最大 999999.99")
	}
	parts := strings.SplitN(s, ".", 2)
	major, _ := strconv.ParseInt(parts[0], 10, 64)
	var minor int64
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
		minor, _ = strconv.ParseInt(fraction, 10, 64)
	}
	cents := major*100 + minor
	if cents < 1 {
		return 0, errors.New("金额必须大于零")
	}
	return cents, nil
}
func Money(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }

var noticePattern = regexp.MustCompile(`^(.{1,100}?)通过扫码向你付款((?:0|[1-9][0-9]{0,5})(?:\.[0-9]{1,2})?)元[。！!]?\s*$`)

func ParseNotice(text string) (string, int64, error) {
	m := noticePattern.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return "", 0, errors.New("不符合支付宝扫码付款通知格式")
	}
	amount, err := ParseMoney(m[2])
	return m[1], amount, err
}

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New("URL 必须为不带用户名和片段的完整 HTTP(S) 地址")
	}
	if len(raw) > 2048 {
		return errors.New("URL 过长")
	}
	return nil
}

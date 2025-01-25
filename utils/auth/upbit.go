package auth

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// GenerateJWT : Upbit용 JWT 생성
// - accessKey: 업비트 Access Key
// - secretKey: 업비트 Secret Key
// - params: 쿼리 파라미터 (없을 경우 nil 또는 빈 map 전달)
func GenerateJWT(accessKey string, secretKey string, params map[string]interface{}) (string, error) {
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.Itoa(rand.Intn(100000))
	claims := jwt.MapClaims{
		"access_key": accessKey,
		"nonce":      nonce,
	}

	if len(params) > 0 {
		claims["query_hash"] = MakeQueryHash(params)
		claims["query_hash_alg"] = "SHA512"
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(secretKey))
	if err != nil {
		return "", err
	}
	return signedToken, nil
}

// MakeQueryHash : Helper (쿼리 스트링 해시가 필요한 경우, GenerateJWT 내에서 처리하지만
// 만약 수동으로 해시를 만들거나 파라미터 구조를 다룰 일이 있을 때 참조)
func MakeQueryHash(params map[string]interface{}) string {
	if len(params) == 0 {
		return ""
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queryParts []string
	for _, k := range keys {
		// url.QueryEscape 사용 시 문서 예시와 다를 수 있으나, Upbit는 일반 form 인코딩도 인식
		// 정확하게는 Upbit가 자동 디코딩함. 문제가 된다면 QueryEscape를 적용하거나, 안 하거나 통일해야 함
		queryParts = append(queryParts, fmt.Sprintf("%s=%s", k, params[k]))
	}
	queryString := strings.Join(queryParts, "&")

	hash := sha512.New()
	hash.Write([]byte(queryString))
	return hex.EncodeToString(hash.Sum(nil))
}

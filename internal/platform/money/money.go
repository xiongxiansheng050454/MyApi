package money

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// Round8 四舍五入到 8 位小数（金额/单价精度）。
func Round8(v float64) float64 { return math.Round(v*1e8) / 1e8 }

// RoundMicro 转成微单位（1e8 = 1）。
func RoundMicro(v float64) int64 { return int64(math.Round(v * 1e8)) }

func MicroToAmount(micro int64) float64 { return float64(micro) / 1e8 }

func AmountToMicro(v float64) int64 { return int64(math.Round(v * 1e8)) }

func Format6(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }
func Format8(v float64) string { return strconv.FormatFloat(v, 'f', 8, 64) }

func ParseNonNegative(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, false
	}
	return v, true
}

func ParsePositive(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0, errors.New("金额必须为大于 0 的数字")
	}
	return v, nil
}

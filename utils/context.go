package utils

type ContextKey uint8

const (
	ContextKeyLogger ContextKey = iota
	ContextKeyCliContext
	ContextKeyAbortFunc
	ContextKeyCfgChan
)

const (
	CfgLotteryChance = "lottery-chance"
	CfxQualityLevel  = "quality-level"
)

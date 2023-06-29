package utils

type ContextKey uint8

const (
	ContextKeyLogger ContextKey = iota
	ContextKeyCliContext
	ContextKeyAbortFunc
	ContextKeyCfgChan
)

const (
	CfgLotteryChance     = "lottery-chance"
	CfgQualityLevel      = "quality-level"
	CfgBlockList         = "block-list"
	CfgBlockListSwitcher = "block-list-switcher"
)

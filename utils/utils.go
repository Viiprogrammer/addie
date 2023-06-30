package utils

type ContextKey uint8

const (
	ContextKeyLogger ContextKey = iota
	ContextKeyCliContext
	ContextKeyAbortFunc
	ContextKeyCfgChan
)

const (
	FbReqTmruestTimer ContextKey = iota
)

const (
	FbReqTmrBeforeRoute ContextKey = iota
	FbReqTmrPreCond
	FbReqTmrBlocklist
	FbReqTmrFakeQuality
	FbReqTmrConsulLottery
	FbReqTmrReqSign
)

const (
	CfgLotteryChance     = "lottery-chance"
	CfgQualityLevel      = "quality-level"
	CfgBlockList         = "block-list"
	CfgBlockListSwitcher = "block-list-switcher"
)

const (
	ChunkPath = iota + 1
	ChunkTitleId
	ChunkEpisodeId
	ChunkQualityLevel
	ChunkName
)

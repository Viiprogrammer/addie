package utils

type ContextKey uint8

const (
	ContextKeyLogger ContextKey = iota
	ContextKeyCliContext
	ContextKeyAbortFunc
	ContextKeyCfgChan
)

const (
	FbCtxRequestTimer ContextKey = iota
)

const (
	FbCtxReqBeforeRoute ContextKey = iota
	FbCtxReqPreCond
	FbCtxReqBlocklist
	FbCtxReqFakeQuality
	FbCtxReqConsulLottery
	FbCtxReqReqSign
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

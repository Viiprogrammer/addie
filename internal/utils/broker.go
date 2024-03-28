package utils

type BrokerChat uint8

const (
	BChatMain BrokerChat = iota
	BChatConfig
	BChatBlocklist
)

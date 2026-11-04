package uikit

// SizeDefault uses the component's documented default size.
type Size uint8

const (
	SizeDefault Size = iota
	SizeXs
	SizeSm
	SizeMd
	SizeLg
)

type Tone uint8

const (
	ToneNeutral Tone = iota
	ToneSecondary
	ToneTertiary
	ToneAccent
	ToneSuccess
	ToneWarning
	ToneDanger
	ToneMuted
	ToneOnAccent
)

type Align uint8

const (
	AlignStart Align = iota
	AlignEnd
	AlignCenter
)

type Density uint8

const (
	DensityCompact Density = iota
	DensityToolbar
	DensityTitle
)

type Edge uint8

const (
	EdgeTop Edge = iota
	EdgeBottom
)

type Status uint8

const (
	StatusConnected Status = iota
	StatusConnecting
	StatusDisconnected
	StatusError
	StatusRunning
	StatusDone
	StatusPaused
	StatusIntercepted
	StatusPartialIntercepted
	StatusPermissionDenied
	StatusAuthFailed
	StatusPartialFailed
	StatusRevoked
	StatusDisabled
	StatusEnabled
	StatusStopping
	StatusInterrupted
)

type MotionMode uint8

const (
	MotionSystem MotionMode = iota
	MotionReduce
	MotionAllow
)

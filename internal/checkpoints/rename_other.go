//go:build !linux && !darwin

package checkpoints

var exchangeFiles = unsupportedExchangeFiles
var moveExclusive = unsupportedMoveExclusive

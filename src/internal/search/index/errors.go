package index

import "errors"

// ErrVectorLengthMismatch indicates two vectors have different dimensions.
var ErrVectorLengthMismatch = errors.New("vector length mismatch")

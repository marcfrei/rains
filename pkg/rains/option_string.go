// Code generated by "stringer -type=Option"; DO NOT EDIT.

package rains

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[QOMinE2ELatency-1]
	_ = x[QOMinLastHopAnswerSize-2]
	_ = x[QOMinInfoLeakage-3]
	_ = x[QOCachedAnswersOnly-4]
	_ = x[QOExpiredAssertionsOk-5]
	_ = x[QOTokenTracing-6]
	_ = x[QONoVerificationDelegation-7]
	_ = x[QONoProactiveCaching-8]
	_ = x[QOMaxFreshness-9]
}

const _Option_name = "QOMinE2ELatencyQOMinLastHopAnswerSizeQOMinInfoLeakageQOCachedAnswersOnlyQOExpiredAssertionsOkQOTokenTracingQONoVerificationDelegationQONoProactiveCachingQOMaxFreshness"

var _Option_index = [...]uint8{0, 15, 37, 53, 72, 93, 107, 133, 153, 167}

func (i Option) String() string {
	i -= 1
	if i < 0 || i >= Option(len(_Option_index)-1) {
		return "Option(" + strconv.FormatInt(int64(i+1), 10) + ")"
	}
	return _Option_name[_Option_index[i]:_Option_index[i+1]]
}

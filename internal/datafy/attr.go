package datafy

const (
	AttrMode                 = "datafy_mode"
	AttrPerformanceArraySize = "datafy_performance_array_size"
)

const (
	ModeAutoscaling            = "autoscaling"
	ModePerformance            = "performance"
	ModeAutoscalingPerformance = "autoscaling_performance"
)

// Modes are every legal value of datafy_mode.
var Modes = []string{ModeAutoscaling, ModePerformance, ModeAutoscalingPerformance}

// PerformanceModes are the modes an array backs, and so the ones
// datafy_performance_array_size may accompany.
var PerformanceModes = []string{ModePerformance, ModeAutoscalingPerformance}

// PerformanceArraySizes are every legal value of datafy_performance_array_size. Sizes below a
// pair are not among them: that is what a volume with no performance optimization already has.
var PerformanceArraySizes = []int{4, 6, 8}

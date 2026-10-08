package datafy

const (
	AttrAutoscaling     = "datafy_autoscaling"
	AttrPerformance     = "datafy_performance"
	AttrPerformanceTier = "datafy_performance_tier"
)

// PerformanceTiers are every legal value of datafy_performance_tier. Each one is the number of
// volumes backing the array. Tiers below a pair are not among them: that is what a volume with
// no performance optimization already has.
var PerformanceTiers = []int{4, 6, 8}

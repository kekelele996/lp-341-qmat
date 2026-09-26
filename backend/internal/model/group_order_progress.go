package model

// GroupOrderProgress 团检订单按企业和套餐归集出的报告进度。
type GroupOrderProgress struct {
	RequiredCount      int `json:"required_count"`
	ReadyCount         int `json:"ready_count"`
	PendingReportCount int `json:"pending_report_count"`
}

// GroupOrderExaminee 团检订单归集的已登记体检人及其报告状态。
type GroupOrderExaminee struct {
	ExamineeID     uint   `json:"examinee_id"`
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Gender         string `json:"gender"`
	Age            int    `json:"age"`
	RegistrationID *uint  `json:"registration_id"`
	GuideNo        string `json:"guide_no"`
	ReportID       *uint  `json:"report_id"`
	ReportNo       string `json:"report_no"`
	ReportStatus   string `json:"report_status"`
	Ready          bool   `json:"ready"`
}

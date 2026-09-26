package model

import "time"

// GroupOrder 团检订单。
type GroupOrder struct {
	ID                   uint        `gorm:"primaryKey" json:"id"`
	EnterpriseID         uint        `gorm:"index;not null" json:"enterprise_id"`
	PackageID            uint        `gorm:"index;not null" json:"package_id"`
	ExamineeCount        int         `json:"examinee_count"`
	Status               string      `gorm:"size:20;default:pending" json:"status"`
	ReportDeliveryStatus string      `gorm:"size:20;default:pending" json:"report_delivery_status"`
	DeliveredAt          *time.Time  `json:"delivered_at"`
	CreatedAt            time.Time   `json:"created_at"`
	Enterprise           Enterprise  `gorm:"foreignKey:EnterpriseID" json:"enterprise,omitempty"`
	Package              Package     `gorm:"foreignKey:PackageID" json:"package,omitempty"`
	// 以下为报告交付进度（非数据库字段，按 企业+套餐 归集已登记体检人实时计算）。
	ExpectedCount int `gorm:"-" json:"expected_count"` // 应交付人数
	ReadyCount    int `gorm:"-" json:"ready_count"`    // 已就绪（报告已发布）人数
	PendingCount  int `gorm:"-" json:"pending_count"`  // 待出报告人数
}

// OrderKey 团检订单归集维度：企业 + 套餐。
type OrderKey struct {
	EnterpriseID uint
	PackageID    uint
}

// OrderReadiness 按 企业+套餐 归集的报告就绪统计。
type OrderReadiness struct {
	EnterpriseID    uint  `json:"enterprise_id"`
	PackageID       uint  `json:"package_id"`
	RegisteredCount int64 `json:"registered_count"` // 已登记体检人数
	ReadyCount      int64 `json:"ready_count"`      // 报告已发布人数
}

// GroupOrderMember 团检订单归集的体检人报告进度行（一次登记一行）。
type GroupOrderMember struct {
	ExamineeID         uint   `json:"examinee_id"`
	ExamineeName       string `json:"examinee_name"`
	IDCardNo           string `json:"id_card_no"`
	RegistrationID     uint   `json:"registration_id"`
	GuideNo            string `json:"guide_no"`
	RegistrationStatus string `json:"registration_status"`
	ReportID           *uint  `json:"report_id"`
	ReportNo           string `json:"report_no"`
	ReportStatus       string `json:"report_status"`
}

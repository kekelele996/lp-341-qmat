package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/blueship581/gbcheckup/internal/constants"
	"github.com/blueship581/gbcheckup/internal/model"
	"github.com/blueship581/gbcheckup/internal/repository"
	"gorm.io/gorm"
)

// setupGroupOrderFixture 构造企业+套餐+2 人团检订单。
func setupGroupOrderFixture(t *testing.T) (*gorm.DB, *EnterpriseService, *model.GroupOrder) {
	t.Helper()
	db := newTestDB(t)
	ent := model.Enterprise{Name: "测试企业", Contact: "联系人"}
	if err := db.Create(&ent).Error; err != nil {
		t.Fatal(err)
	}
	pkg := model.Package{Name: "年度体检", PackageType: constants.PackageAnnual, Price: 599, Status: constants.PackageActive}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	order := model.GroupOrder{EnterpriseID: ent.ID, PackageID: pkg.ID, ExamineeCount: 2, Status: constants.GroupOrderConfirmed}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewEnterpriseService(
		repository.NewEnterpriseRepository(db), repository.NewGroupOrderRepository(db),
		repository.NewPackageRepository(db), testLogger(),
	)
	return db, svc, &order
}

// addGroupMember 为订单归集维度（企业+套餐）登记一名体检人；reportStatus 为空表示未生成报告。
func addGroupMember(t *testing.T, db *gorm.DB, enterpriseID, packageID uint, idCard, name, reportStatus string) {
	t.Helper()
	examinee := model.Examinee{Name: name, IDCardNo: idCard, SourceType: "group", EnterpriseID: &enterpriseID}
	if err := db.Create(&examinee).Error; err != nil {
		t.Fatal(err)
	}
	reg := model.Registration{
		ExamineeID: examinee.ID, PackageID: packageID,
		GuideNo: "G" + idCard, Status: constants.RegistrationCompleted, RegisteredAt: time.Now(),
	}
	if err := db.Create(&reg).Error; err != nil {
		t.Fatal(err)
	}
	if reportStatus == "" {
		return
	}
	report := model.Report{
		RegistrationID: reg.ID, ExamineeID: examinee.ID,
		ReportNo: "R" + idCard, Status: reportStatus,
	}
	if err := db.Create(&report).Error; err != nil {
		t.Fatal(err)
	}
}

func TestEnterpriseService_DeliverReports(t *testing.T) {
	db, svc, order := setupGroupOrderFixture(t)
	ctx := context.Background()

	// 两人均已登记，但仅一人报告已发布：交付应失败并说明还差 1 人。
	addGroupMember(t, db, order.EnterpriseID, order.PackageID, "110101199001010001", "成员甲", constants.ReportPublished)
	addGroupMember(t, db, order.EnterpriseID, order.PackageID, "110101199001010002", "成员乙", constants.ReportReviewed)

	_, err := svc.DeliverReports(ctx, order.ID)
	if err == nil {
		t.Fatal("expected deliver to fail when reports not all published")
	}
	if !strings.Contains(err.Error(), "还差 1 人") {
		t.Fatalf("error should mention missing count, got: %v", err)
	}

	// 交付失败后订单状态不得变更。
	var after model.GroupOrder
	if err := db.First(&after, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.ReportDeliveryStatus == constants.GroupOrderDeliveryDelivered || after.DeliveredAt != nil {
		t.Fatalf("order must not be marked delivered: %+v", after)
	}

	// 全部发布后交付成功：订单状态、交付状态、交付时间一次性更新。
	if err := db.Model(&model.Report{}).Where("report_no = ?", "R110101199001010002").Update("status", constants.ReportPublished).Error; err != nil {
		t.Fatal(err)
	}
	delivered, err := svc.DeliverReports(ctx, order.ID)
	if err != nil {
		t.Fatalf("DeliverReports() error = %v", err)
	}
	if delivered.Status != constants.GroupOrderDone ||
		delivered.ReportDeliveryStatus != constants.GroupOrderDeliveryDelivered ||
		delivered.DeliveredAt == nil {
		t.Fatalf("delivery fields not updated together: %+v", delivered)
	}
	if delivered.ExpectedCount != 2 || delivered.ReadyCount != 2 || delivered.PendingCount != 0 {
		t.Fatalf("progress mismatch: %+v", delivered)
	}

	// 重复交付：返回同一条完成结果（交付时间不变，不报错）。
	again, err := svc.DeliverReports(ctx, order.ID)
	if err != nil {
		t.Fatalf("repeat deliver should be idempotent, got: %v", err)
	}
	if again.ReportDeliveryStatus != constants.GroupOrderDeliveryDelivered ||
		again.DeliveredAt == nil || !again.DeliveredAt.Equal(*delivered.DeliveredAt) {
		t.Fatalf("repeat deliver should return same result: %+v vs %+v", again, delivered)
	}
}

func TestEnterpriseService_ListOrdersProgress(t *testing.T) {
	db, svc, order := setupGroupOrderFixture(t)
	ctx := context.Background()
	addGroupMember(t, db, order.EnterpriseID, order.PackageID, "110101199001010003", "成员丙", constants.ReportPublished)
	// 其他企业的登记不计入本订单进度。
	addGroupMember(t, db, 999, order.PackageID, "110101199001010004", "外部人员", constants.ReportPublished)

	orders, total, err := svc.ListOrders(ctx, 1, 10)
	if err != nil {
		t.Fatalf("ListOrders() error = %v", err)
	}
	if total != 1 || len(orders) != 1 {
		t.Fatalf("orders = %d/%d, want 1", len(orders), total)
	}
	got := orders[0]
	if got.ExpectedCount != 2 || got.ReadyCount != 1 || got.PendingCount != 1 {
		t.Fatalf("progress = %d/%d/%d, want 2/1/1", got.ExpectedCount, got.ReadyCount, got.PendingCount)
	}

	// 详情应归集本企业本套餐的已登记体检人。
	detail, members, err := svc.GetOrderDetail(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrderDetail() error = %v", err)
	}
	if detail.ReadyCount != 1 || detail.PendingCount != 1 {
		t.Fatalf("detail progress mismatch: %+v", detail)
	}
	if len(members) != 1 || members[0].ExamineeName != "成员丙" || members[0].ReportStatus != constants.ReportPublished {
		t.Fatalf("members mismatch: %+v", members)
	}
}

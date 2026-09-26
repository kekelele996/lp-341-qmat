package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/blueship581/gbcheckup/internal/constants"
	"github.com/blueship581/gbcheckup/internal/model"
	"github.com/blueship581/gbcheckup/internal/repository"
	"github.com/blueship581/gbcheckup/internal/util"
)

func TestEnterpriseService_DeliverReportsRequiresAllPublishedReports(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	enterprise := model.Enterprise{Name: "华信科技"}
	otherEnterprise := model.Enterprise{Name: "其他公司"}
	pkg := model.Package{Name: "年度体检", PackageType: constants.PackageAnnual, Status: constants.PackageActive}
	otherPkg := model.Package{Name: "入职体检", PackageType: constants.PackageEntry, Status: constants.PackageActive}
	if err := db.Create(&enterprise).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherEnterprise).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherPkg).Error; err != nil {
		t.Fatal(err)
	}
	order := model.GroupOrder{
		EnterpriseID: enterprise.ID, PackageID: pkg.ID, ExamineeCount: 3,
		Status: constants.GroupOrderConfirmed, ReportDeliveryStatus: constants.ReportDeliveryPending,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	examinees := []model.Examinee{
		{Name: "甲", IDCardNo: "110101199001010001", SourceType: "group", EnterpriseID: &enterprise.ID},
		{Name: "乙", IDCardNo: "110101199001010002", SourceType: "group", EnterpriseID: &enterprise.ID},
		{Name: "丙", IDCardNo: "110101199001010003", SourceType: "group", EnterpriseID: &enterprise.ID},
	}
	if err := db.Create(&examinees).Error; err != nil {
		t.Fatal(err)
	}
	for i, examinee := range examinees {
		reg := model.Registration{
			ExamineeID: examinee.ID, PackageID: pkg.ID,
			GuideNo: "GUIDE-TEST-00" + string(rune('1'+i)),
			Status:  constants.RegistrationCompleted, RegisteredAt: time.Now(),
		}
		if err := db.Create(&reg).Error; err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			report := model.Report{RegistrationID: reg.ID, ExamineeID: examinee.ID, ReportNo: "REPORT-00" + string(rune('1'+i)), Status: constants.ReportPublished}
			if err := db.Create(&report).Error; err != nil {
				t.Fatal(err)
			}
		}
	}

	noiseExaminees := []model.Examinee{
		{Name: "其他企业同套餐", IDCardNo: "110101199001010004", SourceType: "group", EnterpriseID: &otherEnterprise.ID},
		{Name: "同企业其他套餐", IDCardNo: "110101199001010005", SourceType: "group", EnterpriseID: &enterprise.ID},
	}
	if err := db.Create(&noiseExaminees).Error; err != nil {
		t.Fatal(err)
	}
	noiseRegistrations := []model.Registration{
		{ExamineeID: noiseExaminees[0].ID, PackageID: pkg.ID, GuideNo: "GUIDE-TEST-OTHER-ENT", Status: constants.RegistrationCompleted, RegisteredAt: time.Now()},
		{ExamineeID: noiseExaminees[1].ID, PackageID: otherPkg.ID, GuideNo: "GUIDE-TEST-OTHER-PKG", Status: constants.RegistrationCompleted, RegisteredAt: time.Now()},
	}
	if err := db.Create(&noiseRegistrations).Error; err != nil {
		t.Fatal(err)
	}
	for i, reg := range noiseRegistrations {
		if err := db.Create(&model.Report{RegistrationID: reg.ID, ExamineeID: noiseExaminees[i].ID, ReportNo: "REPORT-OTHER-" + string(rune('1'+i)), Status: constants.ReportPublished}).Error; err != nil {
			t.Fatal(err)
		}
	}

	orderRepo := repository.NewGroupOrderRepository(db)
	svc := NewEnterpriseService(repository.NewEnterpriseRepository(db), orderRepo, repository.NewPackageRepository(db), testLogger())

	details, err := svc.GetOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrder() error = %v", err)
	}
	if details.RequiredCount != 3 || details.ReadyCount != 2 || details.PendingReportCount != 1 || len(details.Examinees) != 3 {
		t.Fatalf("progress = %+v, examinees = %d", details.GroupOrderProgress, len(details.Examinees))
	}

	if _, err := svc.DeliverReports(ctx, order.ID); err == nil {
		t.Fatal("expected delivery to fail before all reports are published")
	} else if appErr, ok := err.(*util.AppError); !ok || appErr.HTTPStatus != 409 {
		t.Fatalf("delivery error = %T %v, want 409 conflict", err, err)
	} else if got := appErr.Message; !strings.Contains(got, "还差 1 人") || !strings.Contains(got, "应交付 3 人") || !strings.Contains(got, "已就绪 2 人") {
		t.Fatalf("delivery message = %q, want shortfall and progress", got)
	}
	var stored model.GroupOrder
	if err := db.First(&stored, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status == constants.GroupOrderDone || stored.ReportDeliveryStatus == constants.ReportDeliveryDelivered || stored.DeliveredAt != nil {
		t.Fatalf("incomplete delivery changed order state: %+v", stored)
	}

	var pendingReg model.Registration
	if err := db.Where("examinee_id = ?", examinees[2].ID).First(&pendingReg).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Report{
		RegistrationID: pendingReg.ID, ExamineeID: examinees[2].ID,
		ReportNo: "REPORT-003", Status: constants.ReportPublished,
	}).Error; err != nil {
		t.Fatal(err)
	}

	completed, err := svc.DeliverReports(ctx, order.ID)
	if err != nil {
		t.Fatalf("DeliverReports() error = %v", err)
	}
	firstDeliveredAt := completed.DeliveredAt
	if completed.Status != constants.GroupOrderDone || completed.ReportDeliveryStatus != constants.ReportDeliveryDelivered || firstDeliveredAt == nil {
		t.Fatalf("completed result invalid: %+v", completed.GroupOrder)
	}

	time.Sleep(10 * time.Millisecond)
	repeated, err := svc.DeliverReports(ctx, order.ID)
	if err != nil {
		t.Fatalf("repeated DeliverReports() error = %v", err)
	}
	if repeated.DeliveredAt == nil || !repeated.DeliveredAt.Equal(*firstDeliveredAt) {
		t.Fatalf("repeated delivery changed delivered_at: first=%v repeated=%v", firstDeliveredAt, repeated.DeliveredAt)
	}
}

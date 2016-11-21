package sqldb_test

import (
	. "autoscaler/db/sqldb"
	"autoscaler/models"

	"code.cloudfoundry.org/lager"
	"github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"
	"time"
)

var _ = Describe("ScalingEngineSqldb", func() {
	var (
		url            string
		logger         lager.Logger
		sdb            *ScalingEngineSQLDB
		err            error
		history        *models.AppScalingHistory
		start          int64
		end            int64
		appId          string
		histories      []*models.AppScalingHistory
		canScale       bool
		activeSchedule *models.ActiveSchedule
		schedules      map[string]string
	)

	BeforeEach(func() {
		logger = lager.NewLogger("history-sqldb-test")
		url = os.Getenv("DBURL")
	})

	Describe("NewHistorySQLDB", func() {
		JustBeforeEach(func() {
			sdb, err = NewScalingEngineSQLDB(url, logger)
		})

		AfterEach(func() {
			if sdb != nil {
				err = sdb.Close()
				Expect(err).NotTo(HaveOccurred())
			}
		})

		Context("when db url is not correct", func() {
			BeforeEach(func() {
				url = "postgres://not-exist-user:not-exist-password@localhost/autoscaler?sslmode=disable"
			})
			It("should error", func() {
				Expect(err).To(BeAssignableToTypeOf(&pq.Error{}))
			})

		})

		Context("when url is correct", func() {
			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(sdb).NotTo(BeNil())
			})
		})
	})

	Describe("SaveScalingHistory", func() {
		BeforeEach(func() {
			sdb, err = NewScalingEngineSQLDB(url, logger)
			Expect(err).NotTo(HaveOccurred())
			cleanScalingHistoryTable()
		})

		AfterEach(func() {
			err = sdb.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("When inserting a scaling history record of an app", func() {
			BeforeEach(func() {
				history = &models.AppScalingHistory{
					AppId:        "an-app-id",
					Timestamp:    111111,
					ScalingType:  models.ScalingTypeDynamic,
					Status:       models.ScalingStatusSucceeded,
					OldInstances: 2,
					NewInstances: 4,
					Reason:       "a reason",
					Message:      "a message",
				}
				err = sdb.SaveScalingHistory(history)
			})

			It("has the scaling history record in database", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(hasScalingHistory("an-app-id", 111111)).To(BeTrue())
			})
		})

		Context("When inserting multiple scaling history records of an app", func() {
			BeforeEach(func() {
				history = &models.AppScalingHistory{
					ScalingType:  models.ScalingTypeSchedule,
					Status:       models.ScalingStatusFailed,
					OldInstances: 2,
					NewInstances: 4,
					Reason:       "a reason",
					Message:      "a message",
					Error:        "an error",
				}
				history.AppId = "an-app-id"
				history.Timestamp = 111111
				err = sdb.SaveScalingHistory(history)
				Expect(err).NotTo(HaveOccurred())

				history.AppId = "an-app-id"
				history.Timestamp = 222222
				err = sdb.SaveScalingHistory(history)
				Expect(err).NotTo(HaveOccurred())

				history.AppId = "another-app-id"
				history.Timestamp = 333333
				err = sdb.SaveScalingHistory(history)
				Expect(err).NotTo(HaveOccurred())

			})

			It("has all the histories in database", func() {

				Expect(hasScalingHistory("an-app-id", 111111)).To(BeTrue())
				Expect(hasScalingHistory("an-app-id", 222222)).To(BeTrue())
				Expect(hasScalingHistory("another-app-id", 333333)).To(BeTrue())

			})
		})

	})

	Describe("RetrieveScalingHistories", func() {

		BeforeEach(func() {
			sdb, err = NewScalingEngineSQLDB(url, logger)
			Expect(err).NotTo(HaveOccurred())
			cleanScalingHistoryTable()

			start = 0
			end = -1
			appId = "an-app-id"
		})

		AfterEach(func() {
			err = sdb.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			history = &models.AppScalingHistory{
				AppId:        "an-app-id",
				OldInstances: 2,
				NewInstances: 4,
				Reason:       "a reason",
				Message:      "a message",
			}

			history.Timestamp = 666666
			history.ScalingType = models.ScalingTypeDynamic
			history.Status = models.ScalingStatusSucceeded
			history.Error = ""
			err = sdb.SaveScalingHistory(history)
			Expect(err).NotTo(HaveOccurred())

			history.Timestamp = 222222
			history.ScalingType = models.ScalingTypeDynamic
			history.Status = models.ScalingStatusFailed
			history.Error = "an error"
			err = sdb.SaveScalingHistory(history)
			Expect(err).NotTo(HaveOccurred())

			history.Timestamp = 555555
			history.ScalingType = models.ScalingTypeSchedule
			history.Status = models.ScalingStatusFailed
			history.Error = "an error"
			err = sdb.SaveScalingHistory(history)
			Expect(err).NotTo(HaveOccurred())

			history.Timestamp = 333333
			history.ScalingType = models.ScalingTypeSchedule
			history.Status = models.ScalingStatusIgnored
			history.Error = ""
			err = sdb.SaveScalingHistory(history)
			Expect(err).NotTo(HaveOccurred())

			histories, err = sdb.RetrieveScalingHistories(appId, start, end)
		})

		Context("When the app has no hisotry", func() {
			BeforeEach(func() {
				appId = "app-id-no-history"
			})

			It("returns empty metrics", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(histories).To(BeEmpty())
			})

		})

		Context("when end time is now (end = -1)", func() {
			BeforeEach(func() {
				start = 333333
				end = -1
			})

			It("returns histories from start time to now", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(histories).To(HaveLen(3))
			})

		})

		Context("when end time is before all the history timestamps", func() {
			BeforeEach(func() {
				start = 111111
				end = 222221
			})

			It("returns empty histories", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(histories).To(BeEmpty())
			})

		})

		Context("when start time is after all the history timestamps", func() {
			BeforeEach(func() {
				start = 777777
				end = 888888
			})

			It("returns empty histories", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(histories).To(BeEmpty())
			})

		})

		Context("when start time > end time", func() {
			BeforeEach(func() {
				start = 555555
				end = 555533
			})

			It("returns empty histories", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(histories).To(BeEmpty())
			})
		})

		Context("when retrieving all the histories( start = 0, end = -1) ", func() {
			It("returns all the histories of the app ordered by timestamp", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(histories).To(Equal([]*models.AppScalingHistory{
					&models.AppScalingHistory{
						AppId:        "an-app-id",
						Timestamp:    222222,
						ScalingType:  models.ScalingTypeDynamic,
						Status:       models.ScalingStatusFailed,
						OldInstances: 2,
						NewInstances: 4,
						Reason:       "a reason",
						Message:      "a message",
						Error:        "an error",
					},
					&models.AppScalingHistory{
						AppId:        "an-app-id",
						Timestamp:    333333,
						ScalingType:  models.ScalingTypeSchedule,
						Status:       models.ScalingStatusIgnored,
						OldInstances: 2,
						NewInstances: 4,
						Reason:       "a reason",
						Message:      "a message",
					},
					&models.AppScalingHistory{
						AppId:        "an-app-id",
						Timestamp:    555555,
						ScalingType:  models.ScalingTypeSchedule,
						Status:       models.ScalingStatusFailed,
						OldInstances: 2,
						NewInstances: 4,
						Reason:       "a reason",
						Message:      "a message",
						Error:        "an error",
					},
					&models.AppScalingHistory{
						AppId:        "an-app-id",
						Timestamp:    666666,
						ScalingType:  models.ScalingTypeDynamic,
						Status:       models.ScalingStatusSucceeded,
						OldInstances: 2,
						NewInstances: 4,
						Reason:       "a reason",
						Message:      "a message",
					}}))
			})
		})

		Context("When retrieving part of the histories", func() {
			BeforeEach(func() {
				start = 333333
				end = 555566
			})

			It("return correct histories", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(histories).To(Equal([]*models.AppScalingHistory{
					&models.AppScalingHistory{
						AppId:        "an-app-id",
						Timestamp:    333333,
						ScalingType:  models.ScalingTypeSchedule,
						Status:       models.ScalingStatusIgnored,
						OldInstances: 2,
						NewInstances: 4,
						Reason:       "a reason",
						Message:      "a message",
					},
					&models.AppScalingHistory{
						AppId:        "an-app-id",
						Timestamp:    555555,
						ScalingType:  models.ScalingTypeSchedule,
						Status:       models.ScalingStatusFailed,
						OldInstances: 2,
						NewInstances: 4,
						Reason:       "a reason",
						Message:      "a message",
						Error:        "an error",
					}}))

			})

		})
	})

	Describe("UpdateScalingCooldownExpireTime", func() {
		BeforeEach(func() {
			sdb, err = NewScalingEngineSQLDB(url, logger)
			Expect(err).NotTo(HaveOccurred())
			cleanScalingCooldownTable()
		})

		AfterEach(func() {
			err = sdb.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			err = sdb.UpdateScalingCooldownExpireTime("an-app-id", 222222)
		})

		Context("when there is no previous app cooldown record", func() {
			It("creates the record", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(hasScalingCooldownRecord("an-app-id", 222222)).To(BeTrue())
			})
		})

		Context("when there is previous app cooldown record", func() {
			BeforeEach(func() {
				err = sdb.UpdateScalingCooldownExpireTime("an-app-id", 111111)
				Expect(err).NotTo(HaveOccurred())
			})

			It("removes the previous record and inserts a new record", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(hasScalingCooldownRecord("an-app-id", 111111)).To(BeFalse())
				Expect(hasScalingCooldownRecord("an-app-id", 222222)).To(BeTrue())
			})
		})
	})

	Describe("CanScaleApp", func() {
		BeforeEach(func() {
			sdb, err = NewScalingEngineSQLDB(url, logger)
			Expect(err).NotTo(HaveOccurred())
			cleanScalingCooldownTable()
		})

		AfterEach(func() {
			err = sdb.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			canScale, err = sdb.CanScaleApp("an-app-id")
		})

		Context("when there is no cooldown record before", func() {
			It("returns true", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(canScale).To(BeTrue())
			})
		})

		Context("when the app is still in cooldown period", func() {
			BeforeEach(func() {
				err = sdb.UpdateScalingCooldownExpireTime("an-app-id", time.Now().Add(10*time.Second).UnixNano())
				Expect(err).NotTo(HaveOccurred())
			})
			It("returns false", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(canScale).To(BeFalse())
			})
		})

		Context("when the app passes cooldown period", func() {
			BeforeEach(func() {
				err = sdb.UpdateScalingCooldownExpireTime("an-app-id", time.Now().Add(-10*time.Second).UnixNano())
				Expect(err).NotTo(HaveOccurred())
			})
			It("returns false", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(canScale).To(BeTrue())
			})
		})
	})

	Describe("GetActiveSchedule", func() {
		BeforeEach(func() {
			sdb, err = NewScalingEngineSQLDB(url, logger)
			Expect(err).NotTo(HaveOccurred())
			err = cleanActiveScheduleTable()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			err = sdb.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			activeSchedule, err = sdb.GetActiveSchedule("an-app-id")
		})

		Context("when there is no active schedule for the given app", func() {
			It("should not error and return nil", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(activeSchedule).To(BeNil())
			})
		})

		Context("when there is active schedule ", func() {
			BeforeEach(func() {
				err = insertActiveSchedule("an-app-id", "an-schedule-id", 2, 10, 5)
				Expect(err).NotTo(HaveOccurred())
			})
			It("return the active schedule", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(activeSchedule).To(Equal(&models.ActiveSchedule{
					ScheduleId:         "an-schedule-id",
					InstanceMin:        2,
					InstanceMax:        10,
					InstanceMinInitial: 5,
				}))
			})
		})
		Context("when there is database error", func() {
			BeforeEach(func() {
				sdb.Close()
			})
			It("should error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("GetActiveSchedules", func() {
		BeforeEach(func() {
			sdb, err = NewScalingEngineSQLDB(url, logger)
			Expect(err).NotTo(HaveOccurred())
			err = cleanActiveScheduleTable()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			err = sdb.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			schedules, err = sdb.GetActiveSchedules()
		})

		Context("when the table is empty", func() {
			It("returns an empty active schedules", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(schedules).To(BeEmpty())
			})
		})

		Context("when the table is not empty", func() {
			BeforeEach(func() {
				err = insertActiveSchedule("app-id-1", "schedule-id-1", 2, 10, 5)
				Expect(err).NotTo(HaveOccurred())
				err = insertActiveSchedule("app-id-2", "schedule-id-2", 5, 9, -1)
				Expect(err).NotTo(HaveOccurred())
				err = insertActiveSchedule("app-id-3", "schedule-id-3", 3, 9, 6)
				Expect(err).NotTo(HaveOccurred())
			})
			It("return all active schedules", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(schedules).To(HaveLen(3))
				Expect(schedules).To(HaveKeyWithValue("app-id-1", "schedule-id-1"))
				Expect(schedules).To(HaveKeyWithValue("app-id-2", "schedule-id-2"))
				Expect(schedules).To(HaveKeyWithValue("app-id-3", "schedule-id-3"))
			})

		})
		Context("when there is database error", func() {
			BeforeEach(func() {
				sdb.Close()
			})
			It("should error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("RemoveActiveSchedule", func() {
		BeforeEach(func() {
			sdb, err = NewScalingEngineSQLDB(url, logger)
			Expect(err).NotTo(HaveOccurred())
			err = cleanActiveScheduleTable()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			err = sdb.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			err = sdb.RemoveActiveSchedule("an-app-id")
		})

		Context("when there is no active schedule in table", func() {
			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when there is active schedule in table", func() {
			BeforeEach(func() {
				insertActiveSchedule("an-app-id", "existing-schedule-id", 3, 6, 0)
			})

			It("should remove the active schedule from table", func() {
				Expect(err).NotTo(HaveOccurred())
				schedule, err := sdb.GetActiveSchedule("an-app-id")
				Expect(err).NotTo(HaveOccurred())
				Expect(schedule).To(BeNil())
			})
		})

		Context("when there is database error", func() {
			BeforeEach(func() {
				sdb.Close()
			})
			It("should error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

	})

	Describe("SetActiveSchedule", func() {
		BeforeEach(func() {
			sdb, err = NewScalingEngineSQLDB(url, logger)
			Expect(err).NotTo(HaveOccurred())
			err = cleanActiveScheduleTable()
			Expect(err).NotTo(HaveOccurred())
			activeSchedule = &models.ActiveSchedule{
				ScheduleId:         "a-schedule-id",
				InstanceMin:        2,
				InstanceMax:        8,
				InstanceMinInitial: 5,
			}
		})

		AfterEach(func() {
			err = sdb.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			err = sdb.SetActiveSchedule("an-app-id", activeSchedule)
		})

		Context("when there is no active schedule in table", func() {
			It("should insert the active schedule", func() {
				Expect(err).NotTo(HaveOccurred())
				schedule, err := sdb.GetActiveSchedule("an-app-id")
				Expect(err).NotTo(HaveOccurred())
				Expect(schedule).To(Equal(activeSchedule))
			})
		})

		Context("when there is existing active schedule in table", func() {
			BeforeEach(func() {
				insertActiveSchedule("an-app-id", "existing-schedule-id", 3, 6, 0)
			})

			It("should remove the existing one and insert the new active schedule", func() {
				Expect(err).NotTo(HaveOccurred())
				schedule, err := sdb.GetActiveSchedule("an-app-id")
				Expect(err).NotTo(HaveOccurred())
				Expect(schedule).To(Equal(activeSchedule))
			})
		})

		Context("when there is database error", func() {
			BeforeEach(func() {
				sdb.Close()
			})
			It("should error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

	})

})
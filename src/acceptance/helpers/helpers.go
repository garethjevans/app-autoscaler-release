package helpers

import (
	"acceptance/config"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/helpers"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	. "github.com/onsi/gomega/gexec"
)

const (
	DaysOfMonth Days = "days_of_month"
	DaysOfWeek       = "days_of_week"
	MB               = 1024 * 1024

	TestBreachDurationSeconds = 60
	TestCoolDownSeconds       = 60

	PolicyPath = "/v1/apps/{appId}/policy"
)

type appSummary struct {
	RunningInstances int `json:"running_instances"`
}

type instanceStats struct {
	MemQuota uint64 `json:"mem_quota"`
	Usage    instanceUsage
}

type instanceUsage struct {
	Mem uint64
	CPU float64
}

type instanceInfo struct {
	State string
	Stats instanceStats
}

type appStats map[string]*instanceInfo

type Days string

type ScalingPolicy struct {
	InstanceMin  int               `json:"instance_min_count"`
	InstanceMax  int               `json:"instance_max_count"`
	ScalingRules []*ScalingRule    `json:"scaling_rules,omitempty"`
	Schedules    *ScalingSchedules `json:"schedules,omitempty"`
}

type ScalingRule struct {
	MetricType            string `json:"metric_type"`
	BreachDurationSeconds int    `json:"breach_duration_secs"`
	Threshold             int64  `json:"threshold"`
	Operator              string `json:"operator"`
	CoolDownSeconds       int    `json:"cool_down_secs"`
	Adjustment            string `json:"adjustment"`
}

type ScalingSchedules struct {
	Timezone              string                  `json:"timezone,omitempty"`
	RecurringSchedules    []*RecurringSchedule    `json:"recurring_schedule,omitempty"`
	SpecificDateSchedules []*SpecificDateSchedule `json:"specific_date,omitempty"`
}

type RecurringSchedule struct {
	StartTime             string `json:"start_time"`
	EndTime               string `json:"end_time"`
	DaysOfWeek            []int  `json:"days_of_week,omitempty"`
	DaysOfMonth           []int  `json:"days_of_month,omitempty"`
	ScheduledInstanceMin  int    `json:"instance_min_count"`
	ScheduledInstanceMax  int    `json:"instance_max_count"`
	ScheduledInstanceInit int    `json:"initial_min_instance_count"`
}

type SpecificDateSchedule struct {
	StartDateTime         string `json:"start_date_time"`
	EndDateTime           string `json:"end_date_time"`
	ScheduledInstanceMin  int    `json:"instance_min_count"`
	ScheduledInstanceMax  int    `json:"instance_max_count"`
	ScheduledInstanceInit int    `json:"initial_min_instance_count"`
}

func Curl(cfg *config.Config, args ...string) (int, []byte, error) {
	curlCmd := helpers.Curl(cfg, append([]string{"--output", "/dev/stderr", "--write-out", "%{http_code}"}, args...)...).Wait(cfg.DefaultTimeoutDuration())
	if curlCmd.ExitCode() != 0 {
		return 0, curlCmd.Err.Contents(), fmt.Errorf("curl failed: exit code %d", curlCmd.ExitCode())
	}
	statusCode, err := strconv.Atoi(string(curlCmd.Out.Contents()))
	if err != nil {
		return 0, curlCmd.Err.Contents(), err
	}
	return statusCode, curlCmd.Err.Contents(), nil
}

func OauthToken(cfg *config.Config) string {
	cmd := cf.Cf("oauth-token")
	Expect(cmd.Wait(cfg.DefaultTimeoutDuration())).To(gexec.Exit(0))
	return strings.TrimSpace(string(cmd.Out.Contents()))
}

func EnableServiceAccess(cfg *config.Config, orgName string) {
	enableServiceAccess := cf.Cf("enable-service-access", cfg.ServiceName, "-o", orgName).Wait(cfg.DefaultTimeoutDuration())
	Expect(enableServiceAccess).To(gexec.Exit(0), fmt.Sprintf("Failed to enable service %s for org %s", cfg.ServiceName, orgName))
}

func DisableServiceAccess(cfg *config.Config, orgName string) {
	enableServiceAccess := cf.Cf("disable-service-access", cfg.ServiceName, "-o", orgName).Wait(cfg.DefaultTimeoutDuration())
	Expect(enableServiceAccess).To(gexec.Exit(0), fmt.Sprintf("Failed to disable service %s for org %s", cfg.ServiceName, orgName))
}

func CheckServiceExists(cfg *config.Config) {
	version := cf.Cf("version").Wait(cfg.DefaultTimeoutDuration())
	Expect(version).To(gexec.Exit(0), "Could not determine cf version")

	var serviceExists *gexec.Session
	if strings.Contains(string(version.Out.Contents()), "version 7") {
		serviceExists = cf.Cf("marketplace", "-e", cfg.ServiceName).Wait(cfg.DefaultTimeoutDuration())
	} else {
		serviceExists = cf.Cf("marketplace", "-s", cfg.ServiceName).Wait(cfg.DefaultTimeoutDuration())
	}

	Expect(serviceExists).To(gexec.Exit(0), fmt.Sprintf("Service offering, %s, does not exist", cfg.ServiceName))
}

func GenerateDynamicScaleOutPolicy(cfg *config.Config, instanceMin, instanceMax int, metricName string, threshold int64) string {
	scalingOutRule := ScalingRule{
		MetricType:            metricName,
		BreachDurationSeconds: TestBreachDurationSeconds,
		Threshold:             threshold,
		Operator:              ">=",
		CoolDownSeconds:       TestCoolDownSeconds,
		Adjustment:            "+1",
	}

	policy := ScalingPolicy{
		InstanceMin:  instanceMin,
		InstanceMax:  instanceMax,
		ScalingRules: []*ScalingRule{&scalingOutRule},
	}
	bytes, err := MarshalWithoutHTMLEscape(policy)
	Expect(err).NotTo(HaveOccurred())

	return string(bytes)
}

func GenerateDynamicScaleInPolicy(cfg *config.Config, instanceMin, instanceMax int, metricName string, threshold int64) string {
	scalingInRule := ScalingRule{
		MetricType:            metricName,
		BreachDurationSeconds: TestBreachDurationSeconds,
		Threshold:             threshold,
		Operator:              "<",
		CoolDownSeconds:       TestCoolDownSeconds,
		Adjustment:            "-1",
	}

	policy := ScalingPolicy{
		InstanceMin:  instanceMin,
		InstanceMax:  instanceMax,
		ScalingRules: []*ScalingRule{&scalingInRule},
	}
	bytes, err := MarshalWithoutHTMLEscape(policy)
	Expect(err).NotTo(HaveOccurred())

	return string(bytes)
}

func GenerateDynamicAndSpecificDateSchedulePolicy(cfg *config.Config, instanceMin, instanceMax int, threshold int64,
	timezone string, startDateTime, endDateTime time.Time,
	scheduledInstanceMin, scheduledInstanceMax, scheduledInstanceInit int) string {
	scalingInRule := ScalingRule{
		MetricType:            "memoryused",
		BreachDurationSeconds: TestBreachDurationSeconds,
		Threshold:             threshold,
		Operator:              "<",
		CoolDownSeconds:       TestCoolDownSeconds,
		Adjustment:            "-1",
	}

	specificDateSchedule := SpecificDateSchedule{
		StartDateTime:         startDateTime.Format("2006-01-02T15:04"),
		EndDateTime:           endDateTime.Format("2006-01-02T15:04"),
		ScheduledInstanceMin:  scheduledInstanceMin,
		ScheduledInstanceMax:  scheduledInstanceMax,
		ScheduledInstanceInit: scheduledInstanceInit,
	}

	policy := ScalingPolicy{
		InstanceMin:  instanceMin,
		InstanceMax:  instanceMax,
		ScalingRules: []*ScalingRule{&scalingInRule},
		Schedules: &ScalingSchedules{
			Timezone:              timezone,
			SpecificDateSchedules: []*SpecificDateSchedule{&specificDateSchedule},
		},
	}

	bytes, err := MarshalWithoutHTMLEscape(policy)
	Expect(err).NotTo(HaveOccurred())

	return strings.TrimSpace(string(bytes))
}

func GenerateDynamicAndRecurringSchedulePolicy(cfg *config.Config, instanceMin, instanceMax int, threshold int64,
	timezone string, startTime, endTime time.Time, daysOfMonthOrWeek Days,
	scheduledInstanceMin, scheduledInstanceMax, scheduledInstanceInit int) string {
	scalingInRule := ScalingRule{
		MetricType:            "memoryused",
		BreachDurationSeconds: TestBreachDurationSeconds,
		Threshold:             threshold,
		Operator:              "<",
		CoolDownSeconds:       TestCoolDownSeconds,
		Adjustment:            "-1",
	}

	recurringSchedule := RecurringSchedule{
		StartTime:             startTime.Format("15:04"),
		EndTime:               endTime.Format("15:04"),
		ScheduledInstanceMin:  scheduledInstanceMin,
		ScheduledInstanceMax:  scheduledInstanceMax,
		ScheduledInstanceInit: scheduledInstanceInit,
	}

	if daysOfMonthOrWeek == DaysOfMonth {
		day := startTime.Day()
		recurringSchedule.DaysOfMonth = []int{day}
	} else {
		day := int(startTime.Weekday())
		if day == 0 {
			day = 7
		}
		recurringSchedule.DaysOfWeek = []int{day}
	}

	policy := ScalingPolicy{
		InstanceMin:  instanceMin,
		InstanceMax:  instanceMax,
		ScalingRules: []*ScalingRule{&scalingInRule},
		Schedules: &ScalingSchedules{
			Timezone:           timezone,
			RecurringSchedules: []*RecurringSchedule{&recurringSchedule},
		},
	}

	bytes, err := MarshalWithoutHTMLEscape(policy)
	Expect(err).NotTo(HaveOccurred())

	return string(bytes)
}

func RunningInstances(appGUID string, timeout time.Duration) int {
	cmd := cf.Cf("curl", "/v2/apps/"+appGUID+"/summary")
	Expect(cmd.Wait(timeout)).To(gexec.Exit(0))

	var summary appSummary
	err := json.Unmarshal(cmd.Out.Contents(), &summary)
	Expect(err).ToNot(HaveOccurred())
	return summary.RunningInstances
}

func WaitForNInstancesRunning(appGUID string, instances int, timeout time.Duration) {
	Eventually(func() int {
		return RunningInstances(appGUID, timeout)
	}, timeout, 10*time.Second).Should(Equal(instances))
}

func allInstancesCPU(appGUID string, timeout time.Duration) []float64 {
	cmd := cf.Cf("curl", "/v2/apps/"+appGUID+"/stats")
	Expect(cmd.Wait(timeout)).To(gexec.Exit(0))

	var stats appStats
	err := json.Unmarshal(cmd.Out.Contents(), &stats)
	Expect(err).ToNot(HaveOccurred())

	if len(stats) == 0 {
		return []float64{}
	}

	cpu := make([]float64, len(stats))

	for k, instance := range stats {
		i, err := strconv.Atoi(k)
		Expect(err).NotTo(HaveOccurred())
		cpu[i] = instance.Stats.Usage.CPU
	}
	return cpu
}

func AverageCPUByInstance(appGUID string, timeout time.Duration) float64 {
	cpuArray := allInstancesCPU(appGUID, timeout)
	instanceCount := len(cpuArray)
	if instanceCount == 0 {
		return math.MaxInt64
	}

	var cpuSum float64
	for _, c := range cpuArray {
		cpuSum += c
	}

	return cpuSum / float64(instanceCount)
}

func allInstancesMemoryUsed(appGUID string, timeout time.Duration) []uint64 {
	cmd := cf.Cf("curl", "/v2/apps/"+appGUID+"/stats")
	Expect(cmd.Wait(timeout)).To(gexec.Exit(0))

	var stats appStats
	err := json.Unmarshal(cmd.Out.Contents(), &stats)
	Expect(err).ToNot(HaveOccurred())

	if len(stats) == 0 {
		return []uint64{}
	}

	mem := make([]uint64, len(stats))

	for k, instance := range stats {
		i, err := strconv.Atoi(k)
		Expect(err).NotTo(HaveOccurred())
		mem[i] = instance.Stats.Usage.Mem
	}
	return mem
}

func AverageMemoryUsedByInstance(appGUID string, timeout time.Duration) uint64 {
	memoryUsedArray := allInstancesMemoryUsed(appGUID, timeout)
	instanceCount := len(memoryUsedArray)
	if instanceCount == 0 {
		return math.MaxInt64
	}

	var memSum uint64
	for _, m := range memoryUsedArray {
		memSum += m
	}

	return memSum / uint64(len(memoryUsedArray))
}

func MarshalWithoutHTMLEscape(v interface{}) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	err := enc.Encode(v)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func CreatePolicy(cfg *config.Config, appName, appGUID, policy string) {
	if cfg.IsServiceOfferingEnabled() {
		instanceName := generator.PrefixedRandomName("autoscaler", "service")
		createService := cf.Cf("create-service", cfg.ServiceName, cfg.ServicePlan, instanceName).Wait(cfg.DefaultTimeoutDuration())
		Expect(createService).To(Exit(0), "failed creating service")

		bindService := cf.Cf("bind-service", appName, instanceName, "-c", policy).Wait(cfg.DefaultTimeoutDuration())
		Expect(bindService).To(Exit(0), "failed binding service to app with a policy ")
	} else {
		CreatePolicyWithAPI(cfg, appGUID, policy)
	}
}

func CreatePolicyWithAPI(cfg *config.Config, appGUID, policy string) {
	oauthToken := OauthToken(cfg)
	client := GetHTTPClient(cfg)

	policyURL := fmt.Sprintf("%s%s", cfg.ASApiEndpoint, strings.Replace(PolicyPath, "{appId}", appGUID, -1))
	req, err := http.NewRequest("PUT", policyURL, bytes.NewBuffer([]byte(policy)))
	Expect(err).ShouldNot(HaveOccurred())
	req.Header.Add("Authorization", oauthToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	Expect(err).ShouldNot(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode == 200 || resp.StatusCode == 201).Should(BeTrue())
	Expect([]int{http.StatusOK, http.StatusCreated}).To(ContainElement(resp.StatusCode))
}

func GetHTTPClient(cfg *config.Config) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
			DisableCompression:  true,
			DisableKeepAlives:   true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.SkipSSLValidation,
			},
		},
		Timeout: 30 * time.Second,
	}
}
func GetAppGuid(cfg *config.Config, appName string) string {
	guid := cf.Cf("app", appName, "--guid").Wait(cfg.DefaultTimeoutDuration())
	Expect(guid).To(Exit(0))
	return strings.TrimSpace(string(guid.Out.Contents()))
}

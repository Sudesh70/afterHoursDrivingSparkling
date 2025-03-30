package main

import (
	"context"
	"encoding/json"
	"fmt"
	"gopkg.in/mail.v2"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"
	"time"
)

// Struct for summary report response
type SummaryReport struct {
	DeviceID     int     `json:"deviceId"`
	DeviceName   string  `json:"deviceName"`
	MaxSpeed     float64 `json:"maxSpeed"`
	AverageSpeed float64 `json:"averageSpeed"`
	Distance     float64 `json:"distance"`
	SpentFuel    float64 `json:"spentFuel"`
	EngineHours  float64 `json:"engineHours"`
}

func main() {
	loginURL := "https://sparkingtracking.com/api/session"
	reportURL := "https://sparkingtracking.com/api/reports/summary"

	username := "admin@tracking.sparklingproperty.com"
	password := "qDO6j5S5Hq1e"

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	err := authenticate(client, loginURL, username, password)
	if err != nil {
		fmt.Println("❌ Authentication failed:", err)
		os.Exit(1)
	}
	fmt.Println("✅ Authentication Successful!")

	location, _ := time.LoadLocation("Australia/Melbourne")

	now := time.Now().In(location)
	yesterday := now.AddDate(0, 0, -1)

	fromTime := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 18, 0, 0, 0, location)
	toTime := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 0, location)

	fromDate := fromTime.Format(time.RFC3339)
	toDate := toTime.Format(time.RFC3339)

	groupIDs := []int{1, 2, 3} // Example group IDs
	var reports []SummaryReport
	var wg sync.WaitGroup
	reportChan := make(chan []SummaryReport)
	errorChan := make(chan error, len(groupIDs))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, groupID := range groupIDs {
		wg.Add(1)
		go func(gID int) {
			defer wg.Done()
			report, err := getSummaryReport(ctx, client, reportURL, fromDate, toDate, gID)
			if err != nil {
				errorChan <- err
				return
			}
			reportChan <- report
		}(groupID)
	}

	go func() {
		wg.Wait()
		close(reportChan)
		close(errorChan)
	}()

	for report := range reportChan {
		reports = append(reports, report...)
	}

	if len(errorChan) > 0 {
		for err := range errorChan {
			fmt.Println("❌ Error fetching report:", err)
		}
		os.Exit(1)
	}

	for i := range reports {
		reports[i].Distance /= 1000 // Convert meters to km
	}

	jsonOutput, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		fmt.Println("❌ Error converting report to JSON:", err)
		os.Exit(1)
	}

	fmt.Println("\n📊 After-Hours Summary Report JSON Output (Distance in KM):")
	fmt.Println(string(jsonOutput))

	emailBody := formatReportHTML(reports, fromDate, toDate)
	err = sendEmail("After-Hours Summary Report", emailBody)
	if err != nil {
		fmt.Println("❌ Error sending email:", err)
	} else {
		fmt.Println("📧 Email sent successfully!")
	}
	fmt.Println("📆 Time Period:", fromDate, "to", toDate)
}

func authenticate(client *http.Client, apiURL, username, password string) error {
	data := url.Values{}
	data.Set("email", username)
	data.Set("password", password)

	req, err := http.NewRequest("POST", apiURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.URL.RawQuery = data.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println("🔍 Authentication Response:", string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func getSummaryReport(ctx context.Context, client *http.Client, apiURL, startTime, endTime string, groupID int) ([]SummaryReport, error) {
	reqURL, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}

	q := reqURL.Query()
	q.Set("from", startTime)
	q.Set("to", endTime)
	q.Set("groupId", fmt.Sprintf("%d", groupID))
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var reportData []SummaryReport
	if err := json.Unmarshal(body, &reportData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v\nResponse: %s", err, string(body))
	}

	return reportData, nil
}

func formatReportHTML(reports []SummaryReport, fromDate, toDate string) string {
	html := fmt.Sprintf(`<h2>After-Hours Summary Report<br><small>%s to %s</small></h2>`, fromDate[:16], toDate[:16])
	html += `<table border="1" cellpadding="5" cellspacing="0"><tr><th>Device Name</th><th>Distance (km)</th><th>Spent Fuel (L)</th><th>Engine Hours</th></tr>`
	for _, r := range reports {
		html += fmt.Sprintf("<tr><td>%s</td><td>%.2f</td><td>%.2f</td><td>%.2f</td></tr>",
			r.DeviceName, r.Distance, r.SpentFuel, r.EngineHours)
	}
	html += "</table>"
	return html
}

func sendEmail(subject, body string) error {
	smtpHost := "mail.sunru.com.au"
	smtpPort := 587
	smtpUser := "support@sunru.com.au"
	smtpPass := "Corona@2020"

	m := mail.NewMessage()
	m.SetHeader("From", smtpUser)
	m.SetHeader("To", "dandydiner@outlook.com")
	//m.SetHeader("To", "malien.n@sunru.com.au", "malien7037@gmail.com")
	m.SetHeader("Cc", "malien.n@sunru.com.au", "malien7037@gmail.com")
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	d := mail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)

	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	fmt.Println("✅ Email sent successfully!")
	return nil
}

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

	groupIDs := []int{1, 2, 3}
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
		reports[i].Distance /= 1000
	}

	emailBody := formatReportHTML(reports, fromDate, toDate)
	if err := sendEmail("After-Hours Summary Report", emailBody); err != nil {
		fmt.Println("❌ Error sending email:", err)
	} else {
		fmt.Println("📧 Email sent successfully!")
	}
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
	fromParsed, _ := time.Parse(time.RFC3339, fromDate)
	toParsed, _ := time.Parse(time.RFC3339, toDate)

	timeRange := fmt.Sprintf("%s to %s",
		fromParsed.Format("02 Jan 2006 15:04"),
		toParsed.Format("02 Jan 2006 15:04"))

	html := `
	<html>
	<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
	<body style="font-family: Arial, sans-serif; background-color: #f7f7f7; padding: 20px;">
		<div style="max-width: 800px; margin: auto; background: white; padding: 20px; border-radius: 10px; box-shadow: 0 2px 5px rgba(0,0,0,0.1);">
			<h2 style="text-align: center; color: #cc0000;">After-Hours  Driving Summary Report </h2>
			<p style="text-align: center; color: #333;">Period: ` + timeRange + `</p>
			<table style="width: 100%; border-collapse: collapse; margin-top: 20px;" border="1">
				<tr style="background-color: #f2f2f2;">
					<th style="padding: 10px;">Device Name</th>
					<th style="padding: 10px;">Distance (km)</th>
					<th style="padding: 10px;">Spent Fuel (L)</th>
					<th style="padding: 10px;">Engine Hours</th>
				</tr>`

	for _, r := range reports {
		style := ""
		if r.Distance > 20 {
			style = "style='color:darkorange; font-weight:bold;'"
		}
		html += fmt.Sprintf(`
			<tr %s style="text-align: center;">
				<td style="padding: 8px;">%s</td>
				<td style="padding: 8px;">%.2f</td>
				<td style="padding: 8px;">%.2f</td>
				<td style="padding: 8px;">%.2f</td>
			</tr>`, style, r.DeviceName, r.Distance, r.SpentFuel, r.EngineHours)
	}

	html += `
			</table>
			<p style="text-align: center; font-size: 12px; color: #888; margin-top: 20px;">© 2025 SunRu Fleet Management</p>
		</div>
	</body>
	</html>`
	return html
}

func sendEmail(subject, body string) error {
	smtpHost := "smtp.titan.email"
	smtpPort := 465 // switched to STARTTLS port
	smtpUser := "info@suntrack.com.au"
	smtpPass := "Dehan@2009228"

	m := mail.NewMessage()
	m.SetAddressHeader("From", smtpUser, "SunTrack-GPS")
	m.SetHeader("To", "dandydiner@outlook.com")
	m.SetHeader("Cc", "malien.n@sunru.com.au")
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	d := mail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)
	return d.DialAndSend(m)
}

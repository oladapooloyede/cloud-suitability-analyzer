/*******************************************************************************
 * Copyright (c) 2018 - Present VMware, Inc. All Rights Reserved.
 * SPDX-License-Identifier: BSD-2
 ******************************************************************************/

package report

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"csa-app/db"
	"csa-app/model"
	"csa-app/util"
)

type ReportService struct {
	reportDataRepository db.ReportDataRepository
	slocRepository       db.SlocRepository
}

func NewReportService(reportDataRepository db.ReportDataRepository, slocRepository db.SlocRepository) *ReportService {
	return &ReportService{
		reportDataRepository: reportDataRepository,
		slocRepository:       slocRepository,
	}
}

func NewReportSvc(mgr *db.Repositories) *ReportService {
	return &ReportService{
		reportDataRepository: mgr.Reports,
		slocRepository:       mgr.Sloc,
	}
}

func (reportService *ReportService) ListReports(reportType *string) {
	var buffer bytes.Buffer

	rType := strings.ToLower(*reportType)
	reports := db.GetAvailableReports()

	typeReports, ok := reports[rType]

	if ok {
		reportService.listReportsForType(rType, typeReports, &buffer)
	} else {
		for rType, availReports := range reports {
			reportService.listReportsForType(rType, availReports, &buffer)
		}
	}
	buffer.WriteString("==============================================\n")

	fmt.Println(buffer.String())
	os.Exit(0)
}

func (reportService *ReportService) listReportsForType(reportType string, reports []model.ReportRef, buffer *bytes.Buffer) {

	buffer.WriteString(fmt.Sprintf("\n================ %s Report Types ================\n", strings.ToUpper(reportType)))

	for _, report := range reports {
		buffer.WriteString(fmt.Sprintf("%d - %s\n", report.ReportNum, report.Summary))
	}
}

func (reportService *ReportService) ExportReport(runId uint, reportId int, title string, displayOnStdOut bool, writeFile bool) {

	//Get Report Metadata
	report := db.GetAvailableReportById(util.APP_NAME, reportId)

	//Get Report Headers!
	reportHeaders, longestfield := getReportHeaders(reportId)
	totalFields := len(reportHeaders)
	reportData, longestfield := getReportData(runId, reportId, totalFields, longestfield)

	if writeFile {
		checkAndCreateReportDir(*util.OutputDir)
		//Get Headers!
		file := createReportFile(runId, report.Title, report.Extension, *util.OutputDir)
		defer file.Close()

		if *util.Verbose {
			fmt.Printf("Writing Report [%s] to [%s]\n", report.Title, file.Name())
		}
		//Write the headers
		cnt := 0
		for _, hdr := range reportHeaders {
			if cnt > 0 {
				fmt.Fprint(file, ",")
			}
			fmt.Fprint(file, hdr)
			cnt++
		}

		fmt.Fprint(file, "\n")

		//Write the body
		for _, line := range reportData {
			cnt := 0
			for _, element := range line {
				if cnt > 0 {
					fmt.Fprint(file, ",")

				}
				fmt.Fprint(file, element)
				cnt++
			}
			fmt.Fprint(file, "\n")
		}
	}

	if displayOnStdOut {
		reportService.DisplayReport(reportHeaders, reportData, title, true)
	}
}

func (reportService *ReportService) GenerateReports(run *model.Run) {

	fmt.Printf("\n<= Generate Reports for RunId [%d] =>\n", run.ID)

	findings := db.GetFindingsByRunAndTag(run.ID, model.API_TAG)

	sort.Sort(model.SortFindingByGroupNameLine(findings))

	for _, reportToRun := range run.Reports {

		switch reportToRun {
		case 1:
			run.StartActivity("3rd")
			util.WriteLog("3rd Party Import Report...", "3rd Party Import Report...\n")
			reportService.generateThirdPartyImportReport(run.ID)
			run.StopActivity("3rd", "3rd Party Import Report...done!", true)
		case 2:
			run.StartActivity("api-sum")
			util.WriteLog("Jave API Usage Report (Summary)...", "Jave API Usage Report (Summary)...\n")
			reportService.generateJavaApiSummaryReport(run.ID, findings)
			run.StopActivity("api-sum", "Jave API Usage Report (Summary)...done!", true)
		case 3:
			run.StartActivity("api-det")
			util.WriteLog("Java API Usage Report (Detailed)...", "Java API Usage Report (Detailed)...\n")
			reportService.generateJavaApiDetailReport(run.ID, findings, util.DomainFlag)
			run.StopActivity("api-det", "Java API Usage Report (Detailed)...done!", true)
		case 4:
			run.StartActivity("annotation")
			util.WriteLog("Annotations Used Report...", "Annotations Used Report...\n")
			reportService.generateAnnotationReport(run.ID)
			run.StopActivity("annotation", "Annotations Used Report...done!", true)
		case 5:
			reportService.GenerateClocReport(run, false)
		}
	}
}

func (reportService *ReportService) generateThirdPartyImportReport(runId uint) {

	findings := db.GetFindingsByRunAndTag(runId, model.THIRD_PARTY_TAG)

	thirdPartyUniq := db.UniqueFinding(findings)
	sort.Strings(thirdPartyUniq)

	//Store Report Data
	for _, res2 := range thirdPartyUniq {
		util.WriteLog("3rd Party Import Report...", "3rd Party Import Report...Found Import: %s\n", res2)
		reportService.reportDataRepository.SaveReportData(&model.ReportData{RunID: runId, ReportID: model.THIRD_PARTY_REPORT_ID, Data1: res2})
	}

	reportService.ExportReport(runId, model.THIRD_PARTY_REPORT_ID, "Third-Party", false, true)
}

func (reportService *ReportService) generateJavaApiSummaryReport(runId uint, findings []model.Finding) {

	apiCalls := make(map[string]int)

	for _, entry := range findings {
		apiCalls[entry.Category] += 1
	}
	for _, res1 := range util.SortedKeys(apiCalls) {
		util.WriteLog("Jave API Usage Report (Summary)...", "Jave API Usage Report (Summary)...API: %s Count: %d\n", res1, apiCalls[res1])
		reportService.reportDataRepository.SaveReportData(&model.ReportData{RunID: runId, ReportID: model.API_SUMMARY_REPORT_ID, Data1: res1, Data2: strconv.Itoa(apiCalls[res1])})
	}

	reportService.ExportReport(runId, model.API_SUMMARY_REPORT_ID, "API-SUMMARY", false, true)

}

func (reportService *ReportService) generateJavaApiDetailReport(runId uint, findings []model.Finding, includeDomainDir *bool) {

	for _, entry := range findings {
		util.WriteLog("Java API Usage Report (Detailed)...", "Java API Usage Report (Detailed)...API: %s\n", entry.Category)
		if *includeDomainDir {
			reportService.reportDataRepository.SaveReportData(&model.ReportData{RunID: runId, ReportID: model.API_DETAILED_REPORT_ID, Data1: entry.Application,
				Data2: entry.Category, Data3: entry.Pattern, Data4: entry.Filename, Data5: fmt.Sprint(entry.Line), Data6: entry.Value, Data7: strconv.Itoa(entry.Effort),
				Data8: entry.Advice})
		} else {
			reportService.reportDataRepository.SaveReportData(&model.ReportData{RunID: runId, ReportID: model.API_DETAILED_REPORT_ID, Data1: "",
				Data2: entry.Category, Data3: entry.Pattern, Data4: entry.Filename, Data5: fmt.Sprint(entry.Line), Data6: entry.Value, Data7: strconv.Itoa(entry.Effort),
				Data8: entry.Advice})
		}
	}
	reportService.ExportReport(runId, model.API_DETAILED_REPORT_ID, "API-DETAIL", false, true)

}

func (reportService *ReportService) generateAnnotationReport(runId uint) {

	findings := db.GetFindingsByRunAndTag(runId, model.ANNOTATION_TAG)

	annotationsUniq := db.UniqueFinding(findings)
	sort.Strings(annotationsUniq)
	for _, res3 := range annotationsUniq {
		util.WriteLog("Annotations Report...", "Annotations Report...%s\n", res3)
		reportService.reportDataRepository.SaveReportData(&model.ReportData{RunID: runId, ReportID: model.ANNOTATIONS_REPORT_ID, Data1: res3})
	}

	reportService.ExportReport(runId, model.ANNOTATIONS_REPORT_ID, "ANNOTATIONS", false, true)
}

func (reportService *ReportService) GenerateClocReport(run *model.Run, displayOnly bool) {

	slocData, _ := reportService.slocRepository.GetSlocForRun(run.ID)

	totalFiles := 0
	totalBlank := 0
	totalComment := 0
	totalCode := 0

	langTotals := make(map[string]*model.ReportData)

	for _, item := range slocData {

		if _, ok := langTotals[item.Lang]; !ok {
			langTotals[item.Lang] = &model.ReportData{RunID: run.ID, ReportID: model.CLOC_REPORT_ID, Data1: item.Lang}
		}

		langTotals[item.Lang].Data2 = addIntToString(langTotals[item.Lang].Data2, item.TotalFiles)
		langTotals[item.Lang].Data3 = addIntToString(langTotals[item.Lang].Data3, item.BlankLines)
		langTotals[item.Lang].Data4 = addIntToString(langTotals[item.Lang].Data4, item.CommentLines)
		langTotals[item.Lang].Data5 = addIntToString(langTotals[item.Lang].Data5, item.CodeLines)

		totalFiles += item.TotalFiles
		totalBlank += item.BlankLines
		totalComment += item.CommentLines
		totalCode += item.CodeLines
	}

	//Write Results to DB!
	for _, item := range langTotals {
		reportService.reportDataRepository.SaveReportData(item)
	}

	reportService.reportDataRepository.SaveReportData(&model.ReportData{RunID: run.ID, ReportID: model.CLOC_REPORT_ID, Data1: model.TOTAL_FIELD,
		Data2: fmt.Sprint(totalFiles), Data3: fmt.Sprint(totalBlank),
		Data4: fmt.Sprint(totalComment), Data5: fmt.Sprint(totalCode)})

	reportService.ExportReport(run.ID, model.CLOC_REPORT_ID, "SLOC SUMMARY", true, !displayOnly)

	if *util.DisplayUnknownExts && len(run.UnknownExts) > 0 {
		fmt.Printf("Note: csa-cloc found the following unknown (???) file extensions => ")
		cnt := 0
		for _, ext := range run.UnknownExts {
			if cnt > 0 {
				fmt.Print(",")
			}
			fmt.Printf("%s", ext)
			cnt++
		}
		fmt.Printf("\n\n")
	}
}

func addIntToString(existingCnt string, addCount int) string {

	currentCnt := 0
	if existingCnt != "" {
		currentCnt, _ = strconv.Atoi(existingCnt)
	}

	return fmt.Sprint(currentCnt + addCount)

}

func checkAndCreateReportDir(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if !strings.Contains(path, string(os.PathSeparator)) {
			path = filepath.Join(".", path)
		}
		os.MkdirAll(path, os.ModePerm)
	}
}

func createReportFile(runId uint, reportName string, extension string, path string) (file *os.File) {
	file, err := os.Create(fmt.Sprintf("%s/%d-%s.%s", path, runId, reportName, extension))
	checkReportError(reportName, err)
	return file
}

func checkReportError(reportName string, err error) {
	if err != nil {
		log.Printf("Failed generating report [%s]! Details: %v\n", reportName, err)
	}
}

func (reportService *ReportService) DisplayReport(headers []string, data [][]string, title string, sortByColumn bool) {

	fieldLens := make(map[string]int)

	//get longest header
	for _, hdr := range headers {
		fieldLens[hdr] = len(hdr) + 1
	}

	if sortByColumn {
		sort.Sort(ByColumn(data))
	}

	for _, line := range data {
		for i := 0; i < len(line); i++ {
			fieldLen := len(line[i]) + 1
			if fieldLen > fieldLens[headers[i]] {
				fieldLens[headers[i]] = fieldLen
			}
		}
	}

	//Length of report
	paddlen := 0

	for _, len := range fieldLens {
		paddlen += len + 1
	}
	paddlen -= 1

	divLength := paddlen/2 - len(title)/2
	leftPad := fmt.Sprint(" " + util.Padd(" ", divLength))
	rightPad := fmt.Sprint(util.Padd(" ", divLength) + "")

	fmt.Printf("\n%s%s%s\n", leftPad, title, rightPad)
	fmt.Print(util.Padd("-", paddlen+2) + "\n")

	//Write the headers
	cnt := 0
	for _, hdr := range headers {
		if cnt == 0 {
			fmt.Print("|")
		}
		fmt.Printf("%"+strconv.Itoa(fieldLens[hdr])+"v|", hdr)
		cnt++
	}

	fmt.Print("\n")

	cnt = 0
	for _, hdr := range headers {
		if cnt == 0 {
			fmt.Print("|")
		}
		fmt.Printf("%s%s", util.Padd("-", fieldLens[hdr]), "|")
		cnt++
	}
	fmt.Print("\n")

	//Write the body
	for _, line := range data {
		for i := 0; i < len(line); i++ {
			if i == 0 {
				fmt.Print("|")
			}
			fmt.Printf("%"+strconv.Itoa(fieldLens[headers[i]])+"v|", line[i])

		}
		fmt.Print("\n")
	}

	//Write Footer
	fmt.Print(util.Padd("-", paddlen+2) + "\n")

}

func getReportHeaders(reportId int) (headers []string, longestHeader int) {

	//Get Report Headers!
	reportHeaders := db.GetHeadersForReport(reportId)

	//get longest header
	for _, hdr := range reportHeaders {
		headers = append(headers, hdr.Name)
		fieldLen := len(hdr.Name)
		if fieldLen > longestHeader {
			longestHeader = fieldLen
		}
	}

	return
}

func getReportData(runId uint, reportId int, headerCnt int, longestField int) (data [][]string, longestDataElement int) {
	//Get Report Data
	reportdata := db.GetReportData(runId, reportId)

	for _, line := range reportdata {
		reflectedLine := reflect.ValueOf(line)
		var linedata []string
		for i := 1; i <= headerCnt; i++ {
			fieldData := reflect.Indirect(reflectedLine).FieldByName(fmt.Sprintf("%s%d", model.DATA_FIELD_PREFIX, i))
			fieldLen := len(fieldData.String())
			if fieldLen > longestField {
				longestField = fieldLen
			}

			linedata = append(linedata, fieldData.String())

		}
		data = append(data, linedata)
	}

	longestDataElement = longestField

	return
}

type ByColumn [][]string

func (line ByColumn) Len() int      { return len(line) }
func (line ByColumn) Swap(i, j int) { line[i], line[j] = line[j], line[i] }
func (line ByColumn) Less(i, j int) bool {
	r1 := line[i]
	r2 := line[j]

	for k := 0; k < len(r1); k++ {
		if strings.Contains(r1[k], model.TOTAL_FIELD) {
			return false
		}
		if r1[k] == r2[k] {
			continue
		}
		return r1[k] < r2[k]
	}
	return false
}

package handles

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"

	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"github.com/xhofe/tache"
)

type Payload struct {
	Test [][]string `json:"thumbsUrl"`
}
type CSVRecord struct {
	FileName string `json:"fileName"`
	FilePath string `json:"filePath"`
}
type DeleteRequest struct {
	FilePath string `json:"filePath"`
	FileName string `json:"fileName"`
}

func DeleteEntryFromCSV(filePath, fileName string, csvFilePath string) error {
	file, err := os.Open(csvFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	var filteredRecords [][]string
	for _, record := range records {
		if record[0] != fileName || record[1] != filePath {
			filteredRecords = append(filteredRecords, record)
		}
	}

	if len(records) == len(filteredRecords) {
		fmt.Println("No matching entry found.")
		return nil
	}
	outputFile, err := os.Create(csvFilePath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	writer := csv.NewWriter(outputFile)
	defer writer.Flush()

	for _, record := range filteredRecords {
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

func CSVToJSONArray(filePath string) ([]CSVRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var jsonRecords []CSVRecord
	for _, record := range records {
		if len(record) >= 2 {
			jsonRecords = append(jsonRecords, CSVRecord{
				FileName: record[0],
				FilePath: record[1],
			})
		}
	}

	return jsonRecords, nil
}
func SaveUploadThumb(c *gin.Context) {

	var req DeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorStrResp(c, "failed to parse request body", 400, true)
		return
	}
	csvFilePath := "data/metadata.csv"
	if err := DeleteEntryFromCSV(req.FilePath, req.FileName, csvFilePath); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func GetUploadThumb(c *gin.Context) {
	csvFilePath := "data/metadata.csv"

	jsonRecords, err := CSVToJSONArray(csvFilePath)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	common.SuccessResp(c, jsonRecords)
}

type TaskInfo struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	State    tache.State `json:"state"`
	Status   string      `json:"status"`
	Progress float64     `json:"progress"`
	Error    string      `json:"error"`
}

func getTaskInfo[T tache.TaskWithInfo](task T) TaskInfo {
	errMsg := ""
	if task.GetErr() != nil {
		errMsg = task.GetErr().Error()
	}
	progress := task.GetProgress()
	// if progress is NaN, set it to 100
	if math.IsNaN(progress) {
		progress = 100
	}
	return TaskInfo{
		ID:       task.GetID(),
		Name:     task.GetName(),
		State:    task.GetState(),
		Status:   task.GetStatus(),
		Progress: progress,
		Error:    errMsg,
	}
}

func getTaskInfos[T tache.TaskWithInfo](tasks []T) []TaskInfo {
	return utils.MustSliceConvert(tasks, getTaskInfo[T])
}

func taskRoute[T tache.TaskWithInfo](g *gin.RouterGroup, manager *tache.Manager[T]) {
	g.GET("/undone", func(c *gin.Context) {
		common.SuccessResp(c, getTaskInfos(manager.GetByState(tache.StatePending, tache.StateRunning,
			tache.StateCanceling, tache.StateErrored, tache.StateFailing, tache.StateWaitingRetry, tache.StateBeforeRetry)))
	})
	g.GET("/done", func(c *gin.Context) {
		common.SuccessResp(c, getTaskInfos(manager.GetByState(tache.StateCanceled, tache.StateFailed, tache.StateSucceeded)))
	})
	g.POST("/info", func(c *gin.Context) {
		tid := c.Query("tid")
		task, ok := manager.GetByID(tid)
		if !ok {
			common.ErrorStrResp(c, "task not found", 404)
			return
		}
		common.SuccessResp(c, getTaskInfo(task))
	})
	g.POST("/cancel", func(c *gin.Context) {
		tid := c.Query("tid")
		manager.Cancel(tid)
		common.SuccessResp(c)
	})
	g.POST("/delete", func(c *gin.Context) {
		tid := c.Query("tid")
		manager.Remove(tid)
		common.SuccessResp(c)
	})
	g.POST("/retry", func(c *gin.Context) {
		tid := c.Query("tid")
		manager.Retry(tid)
		common.SuccessResp(c)
	})
	g.POST("/clear_done", func(c *gin.Context) {
		manager.RemoveByState(tache.StateCanceled, tache.StateFailed, tache.StateSucceeded)
		common.SuccessResp(c)
	})
	g.POST("/clear_succeeded", func(c *gin.Context) {
		manager.RemoveByState(tache.StateSucceeded)
		common.SuccessResp(c)
	})
	g.POST("/retry_failed", func(c *gin.Context) {
		manager.RetryAllFailed()
		common.SuccessResp(c)
	})
}

func SetupTaskRoute(g *gin.RouterGroup) {
	taskRoute(g.Group("/upload"), fs.UploadTaskManager)
	taskRoute(g.Group("/copy"), fs.CopyTaskManager)
	taskRoute(g.Group("/offline_download"), tool.DownloadTaskManager)
	taskRoute(g.Group("/offline_download_transfer"), tool.TransferTaskManager)
}

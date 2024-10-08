package handles

import (
	"encoding/json"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"github.com/xhofe/tache"
	"io/ioutil"
	"math"
)

type DeleteRequest struct {
	FilePath string `json:"filePath"`
	FileName string `json:"fileName"`
}

type Metadata struct {
	FilePath string `json:"filePath"`
	FileName string `json:"fileName"`
}

type MetadataSlice []Metadata

type EntryKey struct {
	FilePath string
	FileName string
}

func DeleteEntryFromJSON(key EntryKey, filePath string) error {
	// 读取 JSON 文件
	fileBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}
	var metadataSlice MetadataSlice
	if err := json.Unmarshal(fileBytes, &metadataSlice); err != nil {
		return err
	}
	var newSlice MetadataSlice
	for _, item := range metadataSlice {
		if item.FilePath != key.FilePath || item.FileName != key.FileName {
			newSlice = append(newSlice, item)
		}
	}
	if len(metadataSlice) == len(newSlice) {
		return nil
	}
	newJSONBytes, err := json.MarshalIndent(newSlice, "", "  ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(filePath, newJSONBytes, 0644); err != nil {
		return err
	}

	return nil
}

func SaveUploadThumb(c *gin.Context) {
	var req DeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorStrResp(c, "failed to parse request body", 400, true)
		return
	}
	jsonFilePath := "data/metadata.json"
	key := EntryKey{FilePath: req.FilePath, FileName: req.FileName}
	if err := DeleteEntryFromJSON(key, jsonFilePath); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func GetUploadThumb(c *gin.Context) {
	jsonFilePath := "data/metadata.json"

	metadataSlice, err := ReadMetadataFromJSON(jsonFilePath)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, metadataSlice)
}
func ReadMetadataFromJSON(filePath string) (MetadataSlice, error) {
	var metadataSlice MetadataSlice
	fileBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(fileBytes, &metadataSlice); err != nil {
		return nil, err
	}
	return metadataSlice, nil
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

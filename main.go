package main

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/parnurzeal/gorequest"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

var address = ":8080"
var log = logrus.New()

var appId string
var appSecret string
var accessToken string
var tagId2TemplateIdMap map[string]string
var dataFilePath = "data.json"
var timeout = 5 * time.Second
var retry = 3

type Tag struct {
	Id    int    `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type UserInfo struct {
	OpenId     string   `json:"openid"`
	Nickname   string   `json:"nickname"`
	HeadImgUrl string   `json:"headimgurl"`
	TagIdList  []string `json:"tagid_list"`
}

func init() {
	readConfig()
	if appId == "" {
		log.Error("公众号appId为空")
		os.Exit(0)
	}
	if appSecret == "" {
		log.Error("公众号appSecret为空")
		os.Exit(0)
	}
}

func main() {
	go autoFlushAccessToken()
	startWebService()
}

//----------------------------------------------------------------------------------------------------------------------

func readConfig() error {
	appId = os.Getenv("APP_ID")
	log.WithFields(logrus.Fields{"appId": len(appId)}).Infof("环境变量配置公众号appId长度")

	appSecret = os.Getenv("APP_SECRET")
	log.WithFields(logrus.Fields{"appSecret": len(appSecret)}).Infof("环境变量配置公众号appSecret长度")

	return nil
}

//----------------------------------------------------------------------------------------------------------------------

func startWebService() {
	log.Info("开始web服务")
	engine := gin.Default()
	engine.GET("/", func(context *gin.Context) {
		context.Header("Content-Type", "text/html; charset=utf-8")
		context.String(200, indexHtmlString)
	})
	engine.GET("/listAllUserInfo", func(context *gin.Context) {
		context.JSON(http.StatusOK, createResponseData(listAllUserInfo()))
	})
	engine.GET("/listAllTag", func(context *gin.Context) {
		context.JSON(http.StatusOK, createResponseData(listAllTag()))
	})
	engine.GET("/getTagId2TemplateIdMap", func(context *gin.Context) {
		context.JSON(http.StatusOK, createResponseData(getTagId2TemplateIdMap(), nil))
	})
	engine.POST("/saveTagId2TemplateIdMap", func(context *gin.Context) {
		tagId2TemplateIdMapString := context.PostForm("tagId2TemplateIdMap")
		log.WithFields(logrus.Fields{"tagId2TemplateIdMap": tagId2TemplateIdMapString}).Info("saveTagId2TemplateIdMap表单参数")
		var t2tMap map[string]string
		err := json.Unmarshal([]byte(tagId2TemplateIdMapString), &t2tMap)
		if err == nil {
			log.WithFields(logrus.Fields{"t2tMap": t2tMap}).Info("反序列化tagId2TemplateIdMap成功")
			context.JSON(http.StatusOK, createResponseData(saveTagId2TemplateIdMap(t2tMap)))
		} else {
			log.WithFields(logrus.Fields{"err": err}).Error("反序列化tagId2TemplateIdMap失败")
			context.JSON(http.StatusOK, createResponseData(nil, err))
		}
	})
	engine.POST("/addTagToUser", func(context *gin.Context) {
		tagId := context.PostForm("tagId")
		openIdsString := context.PostForm("openIds")
		log.WithFields(logrus.Fields{"tagId": tagId, "openIds": openIdsString}).Info("addTagToUser表单参数")
		var openIds []string
		err := json.Unmarshal([]byte(openIdsString), &openIds)
		if err == nil {
			log.WithFields(logrus.Fields{"openIds": openIds}).Info("反序列化openIds成功")
			context.JSON(http.StatusOK, createResponseData(addTagToUser(tagId, openIds)))
		} else {
			log.WithFields(logrus.Fields{"err": err}).Error("反序列化openIds失败")
			context.JSON(http.StatusOK, createResponseData(nil, err))
		}
	})
	engine.POST("/deleteTagFromUser", func(context *gin.Context) {
		tagId := context.PostForm("tagId")
		openIdsString := context.PostForm("openIds")
		log.WithFields(logrus.Fields{"tagId": tagId, "openIds": openIdsString}).Info("deleteTagFromUser表单参数")
		var openIds []string
		err := json.Unmarshal([]byte(openIdsString), &openIds)
		if err == nil {
			log.WithFields(logrus.Fields{"openIds": openIds}).Info("反序列化openIds成功")
			context.JSON(http.StatusOK, createResponseData(deleteTagFromUser(tagId, openIds)))
		} else {
			log.WithFields(logrus.Fields{"err": err}).Error("反序列化openIds失败")
			context.JSON(http.StatusOK, createResponseData(nil, err))
		}
	})
	engine.POST("/sendTemplateByTagId", func(context *gin.Context) {
		tagId := context.PostForm("tagId")
		url := context.PostForm("url")
		dataString := context.PostForm("data")
		log.WithFields(logrus.Fields{"tagId": tagId, "url": url, "data": dataString}).Info("sendTemplateByTagId表单参数")
		var data map[string]string
		err := json.Unmarshal([]byte(dataString), &data)
		if err == nil {
			log.WithFields(logrus.Fields{"data": data}).Info("反序列化data成功")
			context.JSON(http.StatusOK, createResponseData(sendTemplateByTagId(tagId, url, data)))
		} else {
			log.WithFields(logrus.Fields{"err": err}).Error("反序列化data失败")
			context.JSON(http.StatusOK, createResponseData(nil, err))
		}
	})
	engine.Run(address)
	log.Info("结束web服务")
}

func createResponseData(data interface{}, err error) interface{} {
	if err == nil {
		return gin.H{"code": 1, "massage": err, "data": data}
	} else {
		return gin.H{"code": 2, "massage": err, "data": data}
	}
}

//----------------------------------------------------------------------------------------------------------------------

//获取全部用户信息
func listAllUserInfo() ([]UserInfo, error) {
	openIds, err := listAllOpenId()
	if err != nil {
		return nil, err
	}
	return listUserInfo(openIds)
}

//给标签用户发送模板消息
func sendTemplateByTagId(tagId string, url string, dataMap map[string]string) (int, error) {
	templateId, exist := getTagId2TemplateIdMap()[tagId]
	if !exist {
		log.Error("tagId没有对应的templateId")
		return -1, errors.New("tagId没有对应的templateId")
	}
	var data map[string]map[string]string
	for key, value := range dataMap {
		data[key] = map[string]string{"value": value}
	}
	log.WithFields(logrus.Fields{"data": data}).Info("重构模板数据")
	openIds, err := listOpenIdByTagId(tagId)
	if err != nil {
		return -1, err
	}
	failCount := 0
	for i := range openIds {
		success, _ := sendTemplate(openIds[i], templateId, url, data)
		if !success {
			failCount = failCount + 1
		}
	}
	return failCount, nil
}

//----------------------------------------------------------------------------------------------------------------------

func saveTagId2TemplateIdMap(t2tMap map[string]string) (success bool, err error) {
	bytes, err := json.Marshal(t2tMap)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("序列化tagId2TemplateIdMap失败")
		return false, err
	}
	err = ioutil.WriteFile(dataFilePath, bytes, 0644)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("写入数据文件失败")
		return false, err
	}
	tagId2TemplateIdMap = t2tMap
	return true, nil
}

func getTagId2TemplateIdMap() map[string]string {
	if tagId2TemplateIdMap != nil && len(tagId2TemplateIdMap) > 0 {
		return tagId2TemplateIdMap
	}
	file, err := os.Open(dataFilePath)
	if err != nil {
		log.Error("打开数据文件失败")
		return map[string]string{}
	}
	defer file.Close()
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Error("读取数据文件失败")
		return map[string]string{}
	}
	var t2tMap map[string]string
	err = json.Unmarshal(bytes, &t2tMap)
	if err == nil {
		tagId2TemplateIdMap = t2tMap
		log.WithFields(logrus.Fields{"tagId2TemplateIdMap": tagId2TemplateIdMap}).Info("反序列化tagId2TemplateIdMap成功")
	} else {
		tagId2TemplateIdMap = map[string]string{}
		log.WithFields(logrus.Fields{"err": err}).Error("反序列化tagId2TemplateIdMap失败")
	}
	return tagId2TemplateIdMap
}

//----------------------------------------------------------------------------------------------------------------------

//获取全部openId
func listAllOpenId() (openIds []string, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestListAllOpenId()
		if err == nil {
			return analysisListAllOpenId(jsonString)
		}
	}
	return nil, err
}

func analysisListAllOpenId(jsonString string) ([]string, error) {
	if !gjson.Valid(jsonString) {
		log.Error("获取全部openId响应json非法")
		return nil, errors.New("获取全部openId响应json非法")
	}
	result := gjson.Get(jsonString, "data")
	if !result.Exists() {
		log.Error("获取全部openId响应json获取data失败")
		return nil, errors.New("获取全部openId响应json获取data失败")
	}
	result = result.Get("openid")
	if !result.Exists() {
		log.Error("获取全部openId响应json获取openid失败")
		return nil, errors.New("获取全部openId响应json获取openid失败")
	}
	var openIds []string
	err := json.Unmarshal([]byte(result.String()), &openIds)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("反序列化获取全部openId响应json失败")
	} else {
		log.WithFields(logrus.Fields{"openIds": openIds}).Info("获取全部openId成功")
	}
	return openIds, err
}

func requestListAllOpenId() (string, error) {
	request := gorequest.New()
	response, body, errs := request.Get("https://api.weixin.qq.com/cgi-bin/user/get").
		Param("access_token", getAccessToken()).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("获取全部openId请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("获取全部openId请求异常")
	}

	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": len(body)}).Info("获取全部openId请求")
	if response.StatusCode != 200 {
		return "", errors.New("获取全部openId响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//获取用户信息
func listUserInfo(openIds []string) (userInfos []UserInfo, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestListUserInfo(openIds)
		if err == nil {
			return analysisListUserInfo(jsonString)
		}
	}
	return nil, err
}

func analysisListUserInfo(jsonString string) ([]UserInfo, error) {
	if !gjson.Valid(jsonString) {
		log.Error("获取用户信息响应json非法")
		return nil, errors.New("获取用户信息响应json非法")
	}
	userInfoList := gjson.Get(jsonString, "user_info_list")
	if !userInfoList.Exists() {
		log.Error("获取用户信息响应json没有user_info_list属性")
		return nil, errors.New("获取用户信息响应json没有user_info_list属性")
	}
	var userInfos []UserInfo
	err := json.Unmarshal([]byte(userInfoList.String()), &userInfos)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("反序列化获取用户信息响应json失败")
	} else {
		log.WithFields(logrus.Fields{"userInfos": userInfos}).Info("获取用户信息成功")
	}
	return userInfos, err
}

func requestListUserInfo(openIds []string) (string, error) {
	var userList []map[string]interface{}
	for i := range openIds {
		userList = append(userList, map[string]interface{}{"openid": openIds[i], "lang": "zh_CN"})
	}
	log.WithFields(logrus.Fields{"userList": userList}).Info("获取用户信息请求参数")

	request := gorequest.New()
	response, body, errs := request.Post("https://api.weixin.qq.com/cgi-bin/user/info/batchget").
		Set("Content-Type", "application/json;CHARSET=utf-8").
		Param("access_token", getAccessToken()).
		Send(
			map[string]interface{}{
				"user_list": userList,
			}).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("获取用户信息请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("获取用户信息请求异常")
	}

	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": len(body)}).Info("获取用户信息请求")
	if response.StatusCode != 200 {
		return "", errors.New("获取用户信息响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//为用户删标签
func deleteTagFromUser(tagId string, openIds []string) (success bool, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestDeleteTagFromUser(tagId, openIds)
		if err == nil {
			return analysisDeleteTagFromUser(jsonString)
		}
	}
	return false, err
}

func analysisDeleteTagFromUser(jsonString string) (bool, error) {
	if !gjson.Valid(jsonString) {
		log.Error("为用户删标签响应json非法")
		return false, errors.New("为用户删标签响应json非法")
	}
	result := gjson.Get(jsonString, "errcode")
	success := result.Exists() && result.Int() == 0
	log.WithFields(logrus.Fields{"success": success}).Info("为用户删标签结果")
	return success, nil
}

func requestDeleteTagFromUser(tagId string, openIds []string) (string, error) {
	request := gorequest.New()
	response, body, errs := request.Post("https://api.weixin.qq.com/cgi-bin/tags/members/batchuntagging").
		Set("Content-Type", "application/json;CHARSET=utf-8").
		Param("access_token", getAccessToken()).
		Send(
			map[string]interface{}{
				"tagid":       tagId,
				"openid_list": openIds,
			}).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("为用户删标签请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("为用户删标签请求异常")
	}

	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": len(body)}).Info("为用户删标签请求")
	if response.StatusCode != 200 {
		return "", errors.New("为用户删标签n响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//为用户加标签
func addTagToUser(tagId string, openIds []string) (success bool, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestAddTagToUser(tagId, openIds)
		if err == nil {
			return analysisAddTagToUser(jsonString)
		}
	}
	return false, err
}

func analysisAddTagToUser(jsonString string) (bool, error) {
	if !gjson.Valid(jsonString) {
		log.Error("为用户加标签响应json非法")
		return false, errors.New("为用户加标签响应json非法")
	}
	result := gjson.Get(jsonString, "errcode")
	success := result.Exists() && result.Int() == 0
	log.WithFields(logrus.Fields{"success": success}).Info("为用户加标签结果")
	return success, nil
}

func requestAddTagToUser(tagId string, openIds []string) (string, error) {
	request := gorequest.New()
	response, body, errs := request.Post("https://api.weixin.qq.com/cgi-bin/tags/members/batchtagging").
		Set("Content-Type", "application/json;CHARSET=utf-8").
		Param("access_token", getAccessToken()).
		Send(
			map[string]interface{}{
				"tagid":       tagId,
				"openid_list": openIds,
			}).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("为用户加标签请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("为用户加标签请求异常")
	}

	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": len(body)}).Info("为用户加标签请求")
	if response.StatusCode != 200 {
		return "", errors.New("为用户加标签响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//获取标签下openid
func listOpenIdByTagId(tagId string) (openIds []string, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestListOpenIdByTagId(tagId)
		if err == nil {
			return analysisListOpenIdByTagId(jsonString)
		}
	}
	return nil, err
}

func analysisListOpenIdByTagId(jsonString string) ([]string, error) {
	if !gjson.Valid(jsonString) {
		log.Error("获取标签下openid响应json非法")
		return nil, errors.New("获取标签下openid响应json非法")
	}
	result := gjson.Get(jsonString, "data")
	if !result.Exists() {
		log.Error("获取标签下openid响应json没有data属性")
		return nil, errors.New("获取标签下openid响应json没有data属性")
	}
	result = gjson.Get(jsonString, "openid")
	if !result.Exists() {
		log.Error("获取标签下openid响应json没有openid属性")
		return nil, errors.New("获取标签下openid响应json没有openid属性")
	}
	var openIds []string
	err := json.Unmarshal([]byte(result.String()), &openIds)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("反序列化获取标签下openid响应json失败")
	} else {
		log.WithFields(logrus.Fields{"openIds": openIds}).Info("获取标签下openid成功")
	}
	return openIds, err
}

func requestListOpenIdByTagId(tagId string) (string, error) {
	request := gorequest.New()
	response, body, errs := request.Get("https://api.weixin.qq.com/cgi-bin/user/tag/get").
		Set("Content-Type", "application/json;CHARSET=utf-8").
		Param("access_token", getAccessToken()).
		Send(
			map[string]interface{}{
				"tagid": tagId,
			}).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("获取标签下openid请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("获取标签下openid请求异常")
	}

	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": len(body)}).Info("获取标签下openid请求")
	if response.StatusCode != 200 {
		return "", errors.New("获取标签下openid响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//删除标签
func deleteTag(tagId string) (success bool, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestDeleteTag(tagId)
		if err == nil {
			return analysisDeleteTag(jsonString)
		}
	}
	return false, err
}

func analysisDeleteTag(jsonString string) (bool, error) {
	if !gjson.Valid(jsonString) {
		log.Error("删除标签响应json非法")
		return false, errors.New("删除标签响应json非法")
	}
	result := gjson.Get(jsonString, "errcode")
	success := result.Exists() && result.Int() == 0
	log.WithFields(logrus.Fields{"success": success}).Info("删除标签结果")
	return success, nil
}

func requestDeleteTag(tagId string) (string, error) {
	request := gorequest.New()
	response, body, errs := request.Post("https://api.weixin.qq.com/cgi-bin/tags/delete").
		Set("Content-Type", "application/json;CHARSET=utf-8").
		Param("access_token", getAccessToken()).
		Send(
			map[string]interface{}{
				"tag": map[string]interface{}{"id": tagId},
			}).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body, "errs": errs}).Info("删除标签请求")

	if response.StatusCode != 200 {
		return "", errors.New("删除标签响应码异常")
	}
	if errs != nil && len(errs) > 0 {
		return "", errors.New("删除标签请求异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//获取所有标签
func listAllTag() (tags []Tag, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestListAllTag()
		if err == nil {
			return analysisListAllTag(jsonString)
		}
	}
	return nil, err
}

func analysisListAllTag(jsonString string) ([]Tag, error) {
	if !gjson.Valid(jsonString) {
		log.Error("获取所有标签响应json非法")
		return nil, errors.New("获取所有标签响应json非法")
	}
	result := gjson.Get(jsonString, "tags")
	if !result.Exists() {
		log.Error("获取所有标签响应json没有tags属性")
		return nil, errors.New("获取所有标签响应json没有tags属性")
	}
	var tags []Tag
	err := json.Unmarshal([]byte(result.String()), &tags)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("反序列化获取所有标签响应json失败")
	} else {
		log.WithFields(logrus.Fields{"tags": tags}).Info("获取所有标签成功")
	}
	return tags, err
}

func requestListAllTag() (string, error) {
	request := gorequest.New()
	response, body, errs := request.Get("https://api.weixin.qq.com/cgi-bin/tags/get").
		Param("access_token", getAccessToken()).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("获取所有标签请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("获取所有标签请求异常")
	}

	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": len(body)}).Info("获取所有标签请求")
	if response.StatusCode != 200 {
		return "", errors.New("获取所有标签响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//创建标签
func createTag(tag string) (success bool, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestCreateTag(tag)
		if err == nil {
			return analysisCreateTag(jsonString)
		}
	}
	return false, err
}

func analysisCreateTag(jsonString string) (bool, error) {
	if !gjson.Valid(jsonString) {
		log.Error("创建标签响应json非法")
		return false, errors.New("创建标签响应json非法")
	}
	result := gjson.Get(jsonString, "errcode")
	success := !result.Exists()
	log.WithFields(logrus.Fields{"success": success}).Info("创建标签结果")
	return success, nil
}

func requestCreateTag(tag string) (string, error) {
	request := gorequest.New()
	response, body, errs := request.Post("https://api.weixin.qq.com/cgi-bin/tags/create").
		Set("Content-Type", "application/json;CHARSET=utf-8").
		Param("access_token", getAccessToken()).
		Send(
			map[string]interface{}{
				"tag": map[string]interface{}{"name": tag},
			}).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("创建标签请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("创建标签请求异常")
	}

	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": len(body)}).Info("创建标签请求")
	if response.StatusCode != 200 {
		return "", errors.New("创建标签响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//发送模板信息
func sendTemplate(openId string, templateId string, url string, data interface{}) (success bool, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestSendTemplate(openId, templateId, url, data)
		if err == nil {
			return analysisSendTemplate(jsonString)
		}
	}
	return false, err
}

func analysisSendTemplate(jsonString string) (bool, error) {
	if !gjson.Valid(jsonString) {
		log.Error("发送模板响应json非法")
		return false, errors.New("发送模板响应json非法")
	}
	result := gjson.Get(jsonString, "errcode")
	success := result.Exists() && result.Int() == 0
	log.WithFields(logrus.Fields{"success": success}).Info("发送模板结果")
	return success, nil
}

func requestSendTemplate(openId string, templateId string, url string, data interface{}) (string, error) {
	request := gorequest.New()
	response, body, errs := request.Post("https://api.weixin.qq.com/cgi-bin/message/template/send").
		Set("Content-Type", "application/json;CHARSET=utf-8").
		Param("access_token", getAccessToken()).
		Send(
			map[string]interface{}{
				"touser":      openId,
				"template_id": templateId,
				"url":         url,
				"data":        data,
			}).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("发送模板信息请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("发送模板信息请求异常")
	}

	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": len(body)}).Info("发送模板信息请求")
	if response.StatusCode != 200 {
		return "", errors.New("发送模板信息响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//获取accessToken
func getAccessToken() string {
	if accessToken == "" {
		flushAccessToken()
	}
	return accessToken
}

func autoFlushAccessToken() {
	for {
		flushAccessToken()
		time.Sleep(30 * time.Minute)
	}
}

func flushAccessToken() {
	for i := 0; i < retry; i++ {
		jsonString, err := requestAccessToken()
		if err == nil {
			accessToken, _ = analysisAccessToken(jsonString)
			return
		}
	}
}

func analysisAccessToken(jsonString string) (string, error) {
	if !gjson.Valid(jsonString) {
		log.Error("获取accessToken响应json非法")
		return "", errors.New("获取accessToken响应json非法")
	}
	result := gjson.Get(jsonString, "access_token")
	if !result.Exists() {
		log.Error("获取accessToken响应json没有access_token属性")
		return "", errors.New("获取accessToken响应json没有access_token属性")
	}
	token := result.String()
	log.WithFields(logrus.Fields{"accessToken": len(token)}).Info("accessToken长度")
	return token, nil
}

func requestAccessToken() (string, error) {
	request := gorequest.New()
	response, body, errs := request.Get("https://api.weixin.qq.com/cgi-bin/token").
		Param("appid", appId).
		Param("secret", appSecret).
		Param("grant_type", "client_credential").
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("获取accessToken请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("获取accessToken请求异常")
	}

	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": len(body)}).Info("获取accessToken请求")
	if response.StatusCode != 200 {
		return "", errors.New("获取accessToken响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

var indexHtmlString = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>张大妈爬虫</title>
    <link type="text/css" rel="stylesheet" href="//unpkg.com/bootstrap/dist/css/bootstrap.min.css"/>
    <link type="text/css" rel="stylesheet" href="//unpkg.com/bootstrap-vue@latest/dist/bootstrap-vue.min.css"/>
</head>
<body>
<div id="app">

    <b-button-group style="width: 100%">
        <b-button variant="primary" @click="saveSearchCondition">save</b-button>
        <b-button variant="info" @click="listSearchCondition">flush</b-button>
    </b-button-group>
    <b-form-textarea :rows="rows" v-model="searchConditionString" @input="flushRows"></b-form-textarea>

</div>
</body>
<script src="//polyfill.io/v3/polyfill.min.js?features=es2015%2CIntersectionObserver" crossorigin="anonymous"></script>
<script src="//unpkg.com/vue@latest/dist/vue.min.js"></script>
<script src="//unpkg.com/bootstrap-vue@latest/dist/bootstrap-vue.min.js"></script>
<script src="//cdn.bootcss.com/jquery/3.4.1/jquery.min.js"></script>
<script>
    var app = new Vue({
        el: '#app',
        data: {
            searchConditionString: "",
            rows: 1,
        },
        methods: {
            saveSearchCondition: function () {
                if (!window.confirm("确定修改？")) {
                    return
                }
                $.ajax({
                    url: 'saveSearchConditions',
                    type: 'post',
                    data: {"searchConditions": app.searchConditionString},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",

                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.status == 1) {
                            alert('修改成功')
                            app.listSearchCondition()
                        } else {
                            alert('修改失败: ' + data.massage)
                        }
                    }
                });
            },
            listSearchCondition: function () {
                $.ajax({
                    url: 'listSearchCondition',
                    type: 'get',
                    data: {},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",

                    error: ajaxErrorDeal,
                    success: function (data) {
                        app.searchConditionString = JSON.stringify(data.data, null, 2);
                        if (app.searchConditionString == null || app.searchConditionString == "") {
                            app.searchConditionString = "[??]"
                        }
                        app.rows = app.searchConditionString.split("\n").length
                        alert('刷新成功')
                    }
                });
            },
            flushRows: function (text) {
                app.rows = text.split("\n").length
            },
        },
    })

    app.listSearchCondition()

    function ajaxErrorDeal() {
        alert("网络错误!");
    }
</script>
</html>`

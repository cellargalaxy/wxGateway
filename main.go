package main

import (
	"encoding/json"
	"errors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/parnurzeal/gorequest"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"
)

var address = ":8990"
var log = logrus.New()

var timeout = 5 * time.Second
var retry = 3
var secretKey = "secret"
var secret = strconv.FormatFloat(rand.Float64(), 'E', -1, 64)

var token string
var appId string
var appSecret string
var accessToken string

var templateId2TagIdMap map[string]int
var dataFilePath = "data.json"

type Template struct {
	TemplateId string `json:"template_id"`
	Title      string `json:"title"`
}

type Tag struct {
	Id    int    `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type UserInfo struct {
	OpenId    string `json:"openid"`
	Nickname  string `json:"nickname"`
	TagIdList []int  `json:"tagid_list"`
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
	token = os.Getenv("TOKEN")
	log.WithFields(logrus.Fields{"token": len(token)}).Infof("环境变量配置token长度")
	getTemplateId2TagIdMap()
	return nil
}

//----------------------------------------------------------------------------------------------------------------------

func startWebService() {
	log.Info("开始web服务")

	engine := gin.Default()
	store := cookie.NewStore([]byte(secret))
	engine.Use(sessions.Sessions("session_id", store))

	engine.GET("/", func(context *gin.Context) {
		context.Header("Content-Type", "text/html; charset=utf-8")
		context.String(200, indexHtmlString)
	})
	engine.GET("/listAllTemplate", validate, func(context *gin.Context) {
		context.JSON(http.StatusOK, createResponseData(listAllTemplate()))
	})
	engine.GET("/listAllTag", validate, func(context *gin.Context) {
		context.JSON(http.StatusOK, createResponseData(listAllTag()))
	})
	engine.GET("/listAllUserInfo", validate, func(context *gin.Context) {
		context.JSON(http.StatusOK, createResponseData(listAllUserInfo()))
	})
	engine.GET("/getTemplateId2TagIdMap", validate, func(context *gin.Context) {
		context.JSON(http.StatusOK, createResponseData(getTemplateId2TagIdMap()))
	})

	engine.POST("/login", func(context *gin.Context) {
		log.Info("用户登录")
		t := context.Request.FormValue("token")
		if t == token {
			setLogin(context)
			context.JSON(http.StatusOK, createResponseData("login success", nil))
		} else {
			log.WithFields(logrus.Fields{"token": token}).Info("非法token")
			context.JSON(http.StatusOK, createResponseData("illegal token", errors.New("illegal token")))
		}
	})
	engine.POST("/createTag", validate, func(context *gin.Context) {
		tag := context.PostForm("tag")
		log.WithFields(logrus.Fields{"tag": tag}).Info("createTag表单参数")
		context.JSON(http.StatusOK, createResponseData(createTag(tag)))
	})
	engine.POST("/deleteTag", validate, func(context *gin.Context) {
		tagIdString := context.PostForm("tagId")
		log.WithFields(logrus.Fields{"tagId": tagIdString}).Info("deleteTag表单参数")
		tagId, err := strconv.Atoi(tagIdString)
		if err != nil {
			log.Error("tagId参数非法")
			context.JSON(http.StatusOK, createResponseData(nil, err))
			return
		}
		context.JSON(http.StatusOK, createResponseData(deleteTag(tagId)))
	})
	engine.POST("/saveTemplateId2TagIdMap", validate, func(context *gin.Context) {
		templateId2TagIdMapString := context.PostForm("templateId2TagIdMap")
		log.WithFields(logrus.Fields{"templateId2TagIdMap": templateId2TagIdMapString}).Info("saveTemplateId2TagIdMap表单参数")
		var t2tMap map[string]int
		err := json.Unmarshal([]byte(templateId2TagIdMapString), &t2tMap)
		if err == nil {
			log.WithFields(logrus.Fields{"t2tMap": t2tMap}).Info("反序列化templateId2TagIdMap成功")
			context.JSON(http.StatusOK, createResponseData(saveTemplateId2TagIdMap(t2tMap)))
		} else {
			log.WithFields(logrus.Fields{"err": err}).Error("反序列化tag2TemplateMap失败")
			context.JSON(http.StatusOK, createResponseData(nil, err))
		}
	})
	engine.POST("/addTagToUser", validate, func(context *gin.Context) {
		tagIdString := context.PostForm("tagId")
		openIdString := context.PostForm("openId")
		log.WithFields(logrus.Fields{"tagId": tagIdString, "openId": openIdString}).Info("addTagToUser表单参数")
		tagId, err := strconv.Atoi(tagIdString)
		if err != nil {
			log.Error("tagId参数非法")
			context.JSON(http.StatusOK, createResponseData(nil, err))
			return
		}
		context.JSON(http.StatusOK, createResponseData(addTagToUser(tagId, []string{openIdString})))
	})
	engine.POST("/deleteTagFromUser", validate, func(context *gin.Context) {
		tagIdString := context.PostForm("tagId")
		openIdString := context.PostForm("openId")
		log.WithFields(logrus.Fields{"tagId": tagIdString, "openId": openIdString}).Info("deleteTagFromUser表单参数")
		tagId, err := strconv.Atoi(tagIdString)
		if err != nil {
			log.Error("tagId参数非法")
			context.JSON(http.StatusOK, createResponseData(nil, err))
			return
		}
		context.JSON(http.StatusOK, createResponseData(deleteTagFromUser(tagId, []string{openIdString})))
	})
	engine.POST("/sendTemplateByTagId", func(context *gin.Context) {
		templateId := context.PostForm("templateId")
		url := context.PostForm("url")
		dataString := context.PostForm("data")
		log.WithFields(logrus.Fields{"templateId": templateId, "url": url, "data": dataString}).Info("sendTemplateByTagId表单参数")
		var data map[string]string
		err := json.Unmarshal([]byte(dataString), &data)
		if err == nil {
			log.WithFields(logrus.Fields{"data": data}).Info("反序列化data成功")
			context.JSON(http.StatusOK, createResponseData(sendTemplateByTagId(templateId, url, data)))
		} else {
			log.WithFields(logrus.Fields{"err": err}).Error("反序列化data失败")
			context.JSON(http.StatusOK, createResponseData(nil, err))
		}
	})
	engine.Run(address)
	log.Info("结束web服务")
}

func validate(context *gin.Context) {
	if !isLogin(context) {
		context.Abort()
		context.JSON(http.StatusUnauthorized, createResponseData("please login", errors.New("please login")))
	} else {
		context.Next()
	}
}

func setLogin(context *gin.Context) {
	session := sessions.Default(context)
	session.Set(secretKey, secret)
	session.Save()
}

func isLogin(context *gin.Context) bool {
	session := sessions.Default(context)
	sessionSecret := session.Get(secretKey)
	return sessionSecret == secret
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
func sendTemplateByTagId(templateId string, url string, dataMap map[string]string) ([]string, error) {
	tagId, exist := templateId2TagIdMap[templateId]
	if !exist {
		log.Error("templateId没有对应的tagId")
		return nil, errors.New("templateId没有对应的tagId")
	}
	log.WithFields(logrus.Fields{"templateId": templateId}).Info("tagId对应的templateId")
	data := map[string]map[string]string{}
	for key, value := range dataMap {
		data[key] = map[string]string{"value": value}
	}
	log.WithFields(logrus.Fields{"data": data}).Info("重构模板数据")
	openIds, err := listOpenIdByTagId(tagId)
	if err != nil {
		return nil, err
	}
	var failOpenIds []string
	for i := range openIds {
		success, _ := sendTemplate(openIds[i], templateId, url, data)
		if !success {
			failOpenIds = append(failOpenIds, openIds[i])
		}
	}
	return failOpenIds, nil
}

//----------------------------------------------------------------------------------------------------------------------

func saveTemplateId2TagIdMap(t2tMap map[string]int) (success bool, err error) {
	bytes, err := json.Marshal(t2tMap)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("序列化templateId2TagIdMap失败")
		return false, err
	}
	err = writeFileOrCreateIfNotExist(dataFilePath, bytes)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("写入数据文件失败")
		return false, err
	}
	templateId2TagIdMap = t2tMap
	return true, nil
}

func getTemplateId2TagIdMap() (map[string]int, error) {
	if templateId2TagIdMap != nil && len(templateId2TagIdMap) > 0 {
		return templateId2TagIdMap, nil
	}
	jsonString, err := readFileOrCreateIfNotExist(dataFilePath, "{}")
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("读取数据文件失败")
		return nil, err
	}
	log.WithFields(logrus.Fields{"jsonString": jsonString}).Info("读取数据文件")
	var t2tMap map[string]int
	err = json.Unmarshal([]byte(jsonString), &t2tMap)
	if err == nil {
		templateId2TagIdMap = t2tMap
		log.WithFields(logrus.Fields{"templateId2TagIdMap": templateId2TagIdMap}).Info("反序列化templateId2TagIdMap成功")
	} else {
		templateId2TagIdMap = nil
		log.WithFields(logrus.Fields{"err": err}).Error("反序列化templateId2TagIdMap失败")
	}
	return templateId2TagIdMap, nil
}

//----------------------------------------------------------------------------------------------------------------------

func writeFileOrCreateIfNotExist(filePath string, text []byte) error {
	_, err := os.Stat(filePath)
	if err == nil || os.IsExist(err) {
		err = ioutil.WriteFile(filePath, text, 0644)
		if err != nil {
			log.WithFields(logrus.Fields{"err": err}).Error("写入文件失败")
		}
		return err
	}
	return createFile(filePath, text)
}

func readFileOrCreateIfNotExist(filePath string, defaultText string) (string, error) {
	_, err := os.Stat(filePath)
	if err == nil || os.IsExist(err) {
		bytes, err := readFile(filePath)
		if err != nil {
			return "", err
		}
		text := string(bytes)
		log.WithFields(logrus.Fields{"text": text}).Info("读取文件文本")
		return text, err
	}
	err = createFile(filePath, []byte(defaultText))
	return defaultText, err
}

func createFile(filePath string, defaultData []byte) error {
	folderPath, _ := path.Split(filePath)
	log.WithFields(logrus.Fields{"folderPath": folderPath}).Info("文件父文件夹")
	if folderPath != "" {
		err := os.MkdirAll(folderPath, 0666)
		if err != nil {
			log.WithFields(logrus.Fields{"err": err}).Error("创建父文件夹失败")
			return err
		}
	}

	file, err := os.Create(filePath)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("创建文件失败")
		return err
	}
	defer file.Close()
	_, err = file.Write(defaultData)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("写入文件初始文本失败")
	}
	return err
}

func readFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("打开文件失败")
		return nil, err
	}
	defer file.Close()
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Error("读取文件失败")
		return nil, err
	}
	return bytes, err
}

//----------------------------------------------------------------------------------------------------------------------

//获取全部模板
func listAllTemplate() (templates []Template, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestListAllTemplate()
		if err == nil {
			return analysisListAllTemplate(jsonString)
		}
		flushAccessToken()
	}
	return nil, err
}

func analysisListAllTemplate(jsonString string) ([]Template, error) {
	if !gjson.Valid(jsonString) {
		log.Error("获取所有模板响应json非法")
		return nil, errors.New("获取所有模板响应json非法")
	}
	result := gjson.Get(jsonString, "template_list")
	if !result.Exists() {
		log.Error("获取所有模板响应json没有template_list属性")
		return nil, errors.New("获取所有模板响应json没有template_list属性")
	}
	var templates []Template
	err := json.Unmarshal([]byte(result.String()), &templates)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("反序列化获取所有模板响应json失败")
	} else {
		log.WithFields(logrus.Fields{"templates": templates}).Info("获取所有模板成功")
	}
	return templates, err
}

func requestListAllTemplate() (string, error) {
	request := gorequest.New()
	response, body, errs := request.Get("https://api.weixin.qq.com/cgi-bin/template/get_all_private_template").
		Param("access_token", getAccessToken()).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("获取所有模板请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("获取所有模板请求异常")
	}
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body}).Info("获取所有模板请求")
	if response.StatusCode != 200 {
		return "", errors.New("获取所有模板响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//获取全部openId
func listAllOpenId() (openIds []string, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestListAllOpenId()
		if err == nil {
			return analysisListAllOpenId(jsonString)
		}
		flushAccessToken()
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
		log.Error("获取全部openId响应json没有data属性")
		return nil, errors.New("获取全部openId响应json没有data属性")
	}
	result = result.Get("openid")
	if !result.Exists() {
		log.Error("获取全部openId响应json没有openid属性")
		return nil, errors.New("获取全部openId响应json没有openid属性")
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
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body}).Info("获取全部openId请求")
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
		flushAccessToken()
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
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body}).Info("获取用户信息请求")
	if response.StatusCode != 200 {
		return "", errors.New("获取用户信息响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//为用户删标签
func deleteTagFromUser(tagId int, openIds []string) (success bool, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestDeleteTagFromUser(tagId, openIds)
		if err == nil {
			return analysisDeleteTagFromUser(jsonString)
		}
		flushAccessToken()
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
	if !success {
		return false, errors.New("为用户删标签失败")
	}
	return success, nil
}

func requestDeleteTagFromUser(tagId int, openIds []string) (string, error) {
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
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body}).Info("为用户删标签请求")
	if response.StatusCode != 200 {
		return "", errors.New("为用户删标签响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//为用户加标签
func addTagToUser(tagId int, openIds []string) (success bool, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestAddTagToUser(tagId, openIds)
		if err == nil {
			return analysisAddTagToUser(jsonString)
		}
		flushAccessToken()
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
	if !success {
		return false, errors.New("为用户加标签失败")
	}
	return success, nil
}

func requestAddTagToUser(tagId int, openIds []string) (string, error) {
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
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body}).Info("为用户加标签请求")
	if response.StatusCode != 200 {
		return "", errors.New("为用户加标签响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//获取标签下openid
func listOpenIdByTagId(tagId int) (openIds []string, err error) {
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
	result = gjson.Get(result.String(), "openid")
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

func requestListOpenIdByTagId(tagId int) (string, error) {
	request := gorequest.New()
	response, body, errs := request.Post("https://api.weixin.qq.com/cgi-bin/user/tag/get").
		Set("Content-Type", "application/json;CHARSET=utf-8").
		Param("access_token", getAccessToken()).
		Send(
			map[string]interface{}{
				"tagid":       tagId,
				"next_openid": "",
			}).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("获取标签下openid请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("获取标签下openid请求异常")
	}

	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body}).Info("获取标签下openid请求")
	if response.StatusCode != 200 {
		return "", errors.New("获取标签下openid响应码异常")
	}
	return body, nil
}

//----------------------------------------------------------------------------------------------------------------------

//删除标签
func deleteTag(tagId int) (success bool, err error) {
	for i := 0; i < retry; i++ {
		jsonString, err := requestDeleteTag(tagId)
		if err == nil {
			return analysisDeleteTag(jsonString)
		}
		flushAccessToken()
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
	if !success {
		return false, errors.New("删除标签失败")
	}
	return success, nil
}

func requestDeleteTag(tagId int) (string, error) {
	request := gorequest.New()
	response, body, errs := request.Post("https://api.weixin.qq.com/cgi-bin/tags/delete").
		Set("Content-Type", "application/json;CHARSET=utf-8").
		Param("access_token", getAccessToken()).
		Send(
			map[string]interface{}{
				"tag": map[string]interface{}{"id": tagId},
			}).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("删除标签请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("删除标签请求异常")
	}
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body}).Info("删除标签请求")
	if response.StatusCode != 200 {
		return "", errors.New("删除标签请求响应码异常")
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
		flushAccessToken()
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
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body}).Info("获取所有标签请求")
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
		flushAccessToken()
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
	if !success {
		return false, errors.New("创建标签失败")
	}
	return success, nil
}

func requestCreateTag(tag string) (string, error) {
	request := gorequest.New()
	response, body, errs := request.Post("https://api.weixin.qq.com/cgi-bin/tags/create").
		Set("Content-Type", "application/json;CHARSET=utf-8").
		Param("access_token", getAccessToken()).
		Send(
			map[string]interface{}{
				"tag": map[string]string{"name": tag},
			}).
		Timeout(timeout).End()
	log.WithFields(logrus.Fields{"errs": errs}).Info("创建标签请求")
	if errs != nil && len(errs) > 0 {
		return "", errors.New("创建标签请求异常")
	}
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body}).Info("创建标签请求")
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
		flushAccessToken()
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
	if !success {
		return false, errors.New("发送模板失败")
	}
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
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body": body}).Info("发送模板信息请求")
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
	log.WithFields(logrus.Fields{"StatusCode": response.StatusCode, "body长度": len(body)}).Info("获取accessToken请求")
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
    <title>微信公众号</title>
    <link type="text/css" rel="stylesheet" href="//unpkg.com/bootstrap/dist/css/bootstrap.css"/>
    <link type="text/css" rel="stylesheet" href="//unpkg.com/bootstrap-vue@latest/dist/bootstrap-vue.css"/>
</head>
<body>
<div id="loginForm">
    <b-input-group>
        <b-form-input type="password" placeholder="token" v-model="token"></b-form-input>
        <b-button variant="outline-primary" @click="login">login</b-button>
    </b-input-group>
</div>
<hr/>
<div id="allTemplate">
    <b-button-group style="width: 100%">
        <b-button>allTemplate</b-button>
        <b-button variant="info" @click="listAllTemplate">flush</b-button>
    </b-button-group>
    <b-form-textarea :rows="rows" v-model="json" @input="flushRows"></b-form-textarea>
</div>
<div id="allTag">
    <b-button-group style="width: 100%">
        <b-button>allTag</b-button>
        <b-button variant="info" @click="listAllTag">flush</b-button>
    </b-button-group>
    <b-form-textarea :rows="rows" v-model="json" @input="flushRows"></b-form-textarea>
</div>
<div id="allUserInfo">
    <b-button-group style="width: 100%">
        <b-button>allUserInfo</b-button>
        <b-button variant="info" @click="listAllUserInfo">flush</b-button>
    </b-button-group>
    <b-form-textarea :rows="rows" v-model="json" @input="flushRows"></b-form-textarea>
</div>
<div id="templateId2TagIdMap">
    <b-button-group style="width: 100%">
        <b-button>tag-template</b-button>
        <b-button variant="primary" @click="saveTemplateId2TagIdMap">save</b-button>
        <b-button variant="info" @click="getTemplateId2TagIdMap">flush</b-button>
    </b-button-group>
    <b-form-textarea :rows="rows" v-model="json" @input="flushRows"></b-form-textarea>
</div>
<hr/>
<div id="createTag">
    <b-input-group prepend="createTag">
        <b-form-input placeholder="tag" v-model="tag"></b-form-input>
        <b-input-group-append>
            <b-button variant="primary" @click="createTag">create</b-button>
        </b-input-group-append>
    </b-input-group>
</div>
<div id="deleteTag">
    <b-input-group prepend="deleteTag">
        <b-form-input placeholder="tagId" v-model="tagId"></b-form-input>
        <b-input-group-append>
            <b-button variant="danger" @click="deleteTag">delete</b-button>
        </b-input-group-append>
    </b-input-group>
</div>

<div id="addTagToUser">
    <b-input-group prepend="addTagToUser">
        <b-form-input placeholder="tagId" v-model="tagId"></b-form-input>
        <b-form-input placeholder="openId" v-model="openId"></b-form-input>
        <b-input-group-append>
            <b-button variant="primary" @click="addTagToUser">add</b-button>
        </b-input-group-append>
    </b-input-group>
</div>
<div id="deleteTagFromUser">
    <b-input-group prepend="deleteTagFromUser">
        <b-form-input placeholder="tagId" v-model="tagId"></b-form-input>
        <b-form-input placeholder="openId" v-model="openId"></b-form-input>
        <b-input-group-append>
            <b-button variant="primary" @click="deleteTagFromUser">delete</b-button>
        </b-input-group-append>
    </b-input-group>
</div>
<hr/>
<div id="allTagUerInfo">
    <b-button-group style="width: 100%">
        <b-button>allTagUerInfo</b-button>
        <b-button variant="info" @click="listAllTagUerInfo">flush</b-button>
    </b-button-group>
    <b-form-textarea :rows="rows" v-model="json" @input="flushRows"></b-form-textarea>
</div>
<hr/>
<div id="sendTemplateByTagId">
    <b-input-group prepend="sendTemplateByTagId">
        <b-form-input placeholder="templateId" v-model="templateId"></b-form-input>
        <b-form-input placeholder="url" v-model="url"></b-form-input>
        <b-input-group-append>
            <b-button variant="primary" @click="sendTemplateByTagId">send</b-button>
        </b-input-group-append>
    </b-input-group>
    <b-form-textarea :rows="rows" v-model="data" placeholder="data" @input="flushRows"></b-form-textarea>
</div>
</body>
<script src="//polyfill.io/v3/polyfill.js?features=es2015%2CIntersectionObserver" crossorigin="anonymous"></script>
<script src="//unpkg.com/vue@latest/dist/vue.js"></script>
<script src="//unpkg.com/bootstrap-vue@latest/dist/bootstrap-vue.js"></script>
<script src="//cdn.bootcss.com/jquery/3.4.1/jquery.js"></script>
<script>
    var loginForm = new Vue({
        el: '#loginForm',
        data: {
            token: "",
        },
        methods: {
            login: function () {
                $.ajax({
                    url: 'login',
                    type: 'post',
                    data: {"token": loginForm.token},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            alert('登录成功')
                            allTemplate.listAllTemplate()
                            allTag.listAllTag()
                            allUserInfo.listAllUserInfo()
                            templateId2TagIdMap.getTemplateId2TagIdMap()
                        } else {
                            alert('登录失败: ' + JSON.stringify(data.massage))
                        }
                    }
                });
            },
        },
    })

    var allTemplate = new Vue({
        el: '#allTemplate',
        data: {
            json: "",
            rows: 1,
        },
        methods: {
            listAllTemplate: function () {
                $.ajax({
                    url: 'listAllTemplate',
                    type: 'get',
                    data: {},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            allTemplate.json = JSON.stringify(data.data, null, 2)
                        } else {
                            allTemplate.json = JSON.stringify(data.massage)
                        }
                        if (allTemplate.json == null) {
                            allTemplate.json = ""
                        }
                        allTemplate.rows = allTemplate.json.split("\n").length
                    }
                });
            },
            flushRows: function (text) {
                allTemplate.rows = text.split("\n").length
            },
        },
    })

    var allTag = new Vue({
        el: '#allTag',
        data: {
            json: "",
            rows: 1,
        },
        methods: {
            listAllTag: function () {
                $.ajax({
                    url: 'listAllTag',
                    type: 'get',
                    data: {},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            allTag.json = JSON.stringify(data.data, null, 2);
                        } else {
                            allTag.json = JSON.stringify(data.massage)
                        }
                        if (allTag.json == null) {
                            allTag.json = ""
                        }
                        allTag.rows = allTag.json.split("\n").length
                    }
                });
            },
            flushRows: function (text) {
                allTag.rows = text.split("\n").length
            },
        },
    })

    var allUserInfo = new Vue({
        el: '#allUserInfo',
        data: {
            json: "",
            rows: 1,
        },
        methods: {
            listAllUserInfo: function () {
                $.ajax({
                    url: 'listAllUserInfo',
                    type: 'get',
                    data: {},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            allUserInfo.json = JSON.stringify(data.data, null, 2);
                        } else {
                            allUserInfo.json = JSON.stringify(data.massage)
                        }
                        if (allUserInfo.json == null) {
                            allUserInfo.json = ""
                        }
                        allUserInfo.rows = allUserInfo.json.split("\n").length
                    }
                });
            },
            flushRows: function (text) {
                allUserInfo.rows = text.split("\n").length
            },
        },
    })

    var templateId2TagIdMap = new Vue({
        el: '#templateId2TagIdMap',
        data: {
            json: "",
            rows: 1,
        },
        methods: {
            saveTemplateId2TagIdMap: function () {
                if (!window.confirm("saveTemplateId2TagIdMap？")) {
                    return
                }
                $.ajax({
                    url: 'saveTemplateId2TagIdMap',
                    type: 'post',
                    data: {"templateId2TagIdMap": templateId2TagIdMap.json},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            alert('修改成功')
                            templateId2TagIdMap.getTemplateId2TagIdMap()
                        } else {
                            alert('修改失败: ' + JSON.stringify(data.massage))
                        }
                    }
                });
            },
            getTemplateId2TagIdMap: function () {
                $.ajax({
                    url: 'getTemplateId2TagIdMap',
                    type: 'get',
                    data: {},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            templateId2TagIdMap.json = JSON.stringify(data.data, null, 2);
                        } else {
                            templateId2TagIdMap.json = JSON.stringify(data.massage)
                        }
                        if (templateId2TagIdMap.json == null) {
                            templateId2TagIdMap.json = ""
                        }
                        templateId2TagIdMap.rows = templateId2TagIdMap.json.split("\n").length
                    }
                });
            },
            flushRows: function (text) {
                templateId2TagIdMap.rows = text.split("\n").length
            },
        },
    })

    var createTag = new Vue({
        el: '#createTag',
        data: {
            tag: "",
        },
        methods: {
            createTag: function () {
                if (!window.confirm("createTag？")) {
                    return
                }
                $.ajax({
                    url: 'createTag',
                    type: 'post',
                    data: {"tag": createTag.tag},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            alert('创建标签成功')
                            createTag.tag = ""
                            allTag.listAllTag()
                        } else {
                            alert('创建标签失败: ' + JSON.stringify(data.massage))
                        }
                    }
                });
            },
        },
    })

    var deleteTag = new Vue({
        el: '#deleteTag',
        data: {
            tagId: "",
        },
        methods: {
            deleteTag: function () {
                if (!window.confirm("deleteTag？")) {
                    return
                }
                $.ajax({
                    url: 'deleteTag',
                    type: 'post',
                    data: {"tagId": deleteTag.tagId},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            alert('删除标签成功')
                            deleteTag.tagId = ""
                            allTag.listAllTag()
                            allUserInfo.listAllUserInfo()
                        } else {
                            alert('删除标签失败: ' + JSON.stringify(data.massage))
                        }
                    }
                });
            },
        },
    })

    var addTagToUser = new Vue({
        el: '#addTagToUser',
        data: {
            tagId: "",
            openId: "",
        },
        methods: {
            addTagToUser: function () {
                if (!window.confirm("addTagToUser？")) {
                    return
                }
                $.ajax({
                    url: 'addTagToUser',
                    type: 'post',
                    data: {"tagId": addTagToUser.tagId, "openId": addTagToUser.openId},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            alert('为用户加标签成功')
                            addTagToUser.tagId = ""
                            addTagToUser.openId = ""
                            allUserInfo.listAllUserInfo()
                        } else {
                            alert('为用户加标签失败: ' + JSON.stringify(data.massage))
                        }
                    }
                });
            },
        },
    })

    var deleteTagFromUser = new Vue({
        el: '#deleteTagFromUser',
        data: {
            tagId: "",
            openId: "",
        },
        methods: {
            deleteTagFromUser: function () {
                if (!window.confirm("deleteTagFromUser？")) {
                    return
                }
                $.ajax({
                    url: 'deleteTagFromUser',
                    type: 'post',
                    data: {"tagId": deleteTagFromUser.tagId, "openId": deleteTagFromUser.openId},
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            alert('为用户删标签成功')
                            deleteTagFromUser.tagId = ""
                            deleteTagFromUser.openId = ""
                            allUserInfo.listAllUserInfo()
                        } else {
                            alert('为用户删标签失败: ' + JSON.stringify(data.massage))
                        }
                    }
                });
            },
        },
    })

    var allTagUerInfo = new Vue({
        el: '#allTagUerInfo',
        data: {
            json: "",
            rows: 1,
        },
        methods: {
            listAllTagUerInfo: function () {
                const tags = JSON.parse(allTag.json)
                const userInfos = JSON.parse(allUserInfo.json)
                const tagId2UserInfos = {}
                for (let i = 0; i < tags.length; i++) {
                    tagId2UserInfos[tags[i].id] = []
                }
                for (let i = 0; i < userInfos.length; i++) {
                    const userInfo = userInfos[i]
                    for (let j = 0; j < userInfo.tagid_list.length; j++) {
                        const users = tagId2UserInfos[userInfo.tagid_list[j]]
                        if (users != null) {
                            tagId2UserInfos[userInfo.tagid_list[j]].push(userInfo.nickname + ':' + userInfo.openid)
                        }
                    }
                }
                const tag2UserInfos = {}
                for (const tagId in tagId2UserInfos) {
                    for (let i = 0; i < tags.length; i++) {
                        if (tags[i].id == tagId) {
                            tag2UserInfos[tags[i].id + ':' + tags[i].name] = tagId2UserInfos[tagId]
                            break
                        }
                    }
                }
                allTagUerInfo.json = JSON.stringify(tag2UserInfos, null, 2)
                allTagUerInfo.rows = allTagUerInfo.json.split("\n").length
            },
            flushRows: function (text) {
                allTagUerInfo.rows = text.split("\n").length
            },
        },
    })

    var sendTemplateByTagId = new Vue({
        el: '#sendTemplateByTagId',
        data: {
            rows: 1,
            templateId: "",
            url: "",
            data: "",
        },
        methods: {
            sendTemplateByTagId: function () {
                if (!window.confirm("sendTemplateByTagId？")) {
                    return
                }
                $.ajax({
                    url: 'sendTemplateByTagId',
                    type: 'post',
                    data: {
                        "templateId": sendTemplateByTagId.templateId,
                        "url": sendTemplateByTagId.url,
                        "data": sendTemplateByTagId.data
                    },
                    contentType: "application/x-www-form-urlencoded",
                    dataType: "json",
                    error: ajaxErrorDeal,
                    success: function (data) {
                        if (data.code == 1) {
                            alert('给标签用户发送模板消息成功')
                        } else {
                            alert('给标签用户发送模板消息失败: ' + JSON.stringify(data.massage))
                        }
                    }
                });
            },
            flushRows: function (text) {
                sendTemplateByTagId.rows = text.split("\n").length
            },
        },
    })

    function ajaxErrorDeal() {
        alert("网络错误!");
    }
</script>
</html>`

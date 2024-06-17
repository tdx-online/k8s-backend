package main

import (
	"context"
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	"log"
	"net/http"
)

var clientset *kubernetes.Clientset
var metricsClientset *versioned.Clientset

func main() {
	// 加载Kubeconfig文件
	kubeconfig := clientcmd.RecommendedHomeFile
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Failed to load kubeconfig: %v", err)
	}

	// 创建Kubernetes客户端
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// 创建Metrics客户端
	metricsClientset, err = versioned.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Metrics client: %v", err)
	}

	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/cluster-info", getClusterInfo) // 获取集群信息
	r.GET("/cluster-load", getClusterLoad) // 获取集群负载信息

	r.POST("/pod", createPod)                    // 创建Pod
	r.DELETE("/pod/:namespace/:name", deletePod) // 删除Pod
	r.GET("/pods", getPods)                      // 获取所有Pod

	r.POST("/deployment", createDeployment)                    // 创建Deployment
	r.DELETE("/deployment/:namespace/:name", deleteDeployment) // 删除Deployment
	r.GET("/deployments", getDeployments)                      // 获取所有Deployment

	r.POST("/service", createService)                    // 创建Service
	r.DELETE("/service/:namespace/:name", deleteService) // 删除Service
	r.GET("/services", getServices)                      // 获取所有Service的API

	r.POST("/configmap", createConfigMap)                    // 创建ConfigMap
	r.DELETE("/configmap/:namespace/:name", deleteConfigMap) // 删除ConfigMap
	r.GET("/configmaps", getConfigMaps)                      // 获取所有ConfigMap

	err = r.Run(":8792")
	if err != nil {
		return
	}
}

// TODO 在进行操作的时候, namespace不应该访问到系统的namespace

/**
 * 获取集群信息
 */
func getClusterInfo(c *gin.Context) {
	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	services, err := clientset.CoreV1().Services("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	deployments, err := clientset.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 定义结构体用于存储简化后的信息
	type ResourceInfo struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	var nodeInfos []ResourceInfo
	for _, node := range nodes.Items {
		status := "Unknown"
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" {
				status = string(condition.Status)
				break
			}
		}
		nodeInfos = append(nodeInfos, ResourceInfo{Name: node.Name, Status: status})
	}

	var podInfos []ResourceInfo
	for _, pod := range pods.Items {
		podInfos = append(podInfos, ResourceInfo{Name: pod.Name, Status: string(pod.Status.Phase)})
	}

	var serviceInfos []ResourceInfo
	for _, service := range services.Items {
		serviceInfos = append(serviceInfos, ResourceInfo{Name: service.Name, Status: string(service.Spec.Type)})
	}

	var deploymentInfos []ResourceInfo
	for _, deployment := range deployments.Items {
		status := "Unknown"
		if deployment.Status.Replicas == deployment.Status.AvailableReplicas {
			status = "Available"
		} else {
			status = "Unavailable"
		}
		deploymentInfos = append(deploymentInfos, ResourceInfo{Name: deployment.Name, Status: status})
	}

	clusterInfo := gin.H{
		"nodes": gin.H{
			"count": len(nodeInfos),
			"items": nodeInfos,
		},
		"pods": gin.H{
			"count": len(podInfos),
			"items": podInfos,
		},
		"services": gin.H{
			"count": len(serviceInfos),
			"items": serviceInfos,
		},
		"deployments": gin.H{
			"count": len(deploymentInfos),
			"items": deploymentInfos,
		},
	}

	c.JSON(http.StatusOK, clusterInfo)
}

/**
 * 获取集群负载信息
 */
func getClusterLoad(c *gin.Context) {
	nodeMetrics, err := metricsClientset.MetricsV1beta1().NodeMetricses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	podMetrics, err := metricsClientset.MetricsV1beta1().PodMetricses("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 定义结构体用于存储节点负载信息
	type NodeLoadInfo struct {
		Name   string `json:"name"`
		CPU    string `json:"cpu"`
		Memory string `json:"memory"`
	}

	// 定义结构体用于存储Pod负载信息
	type PodLoadInfo struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		CPU       string `json:"cpu"`
		Memory    string `json:"memory"`
	}

	var nodeLoadInfos []NodeLoadInfo
	for _, nodeMetric := range nodeMetrics.Items {
		nodeLoadInfos = append(nodeLoadInfos, NodeLoadInfo{
			Name:   nodeMetric.Name,
			CPU:    nodeMetric.Usage.Cpu().String(),
			Memory: nodeMetric.Usage.Memory().String(),
		})
	}

	var podLoadInfos []PodLoadInfo
	for _, podMetric := range podMetrics.Items {
		for _, container := range podMetric.Containers {
			podLoadInfos = append(podLoadInfos, PodLoadInfo{
				Name:      podMetric.Name,
				Namespace: podMetric.Namespace,
				CPU:       container.Usage.Cpu().String(),
				Memory:    container.Usage.Memory().String(),
			})
		}
	}

	loadInfo := gin.H{
		"nodeMetrics": nodeLoadInfos,
		"podMetrics":  podLoadInfos,
	}

	c.JSON(http.StatusOK, loadInfo)
}

/**
 * 根据JSON创建Pod
 */
func createPod(c *gin.Context) {
	var pod v1.Pod

	//if err := c.ShouldBindJSON(&pod); err != nil {
	//	if err := c.ShouldBindYAML(&pod); err != nil {
	//		if err := c.ShouldBindBodyWithJSON(&pod); err != nil {
	//			if err := c.ShouldBindBodyWithYAML(&pod); err != nil {
	//				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	//				return
	//			}
	//		}
	//	}
	//}

	contentType := c.Request.Header.Get("Content-Type")
	switch contentType {
	case "application/json":
		if err := c.ShouldBindJSON(&pod); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	case "application/x-yaml":
		if err := c.ShouldBindBodyWith(&pod, binding.YAML); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	default:
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Unsupported content type"})
		return
	}

	if pod.Namespace == "" {
		pod.Namespace = "default"
	}

	createdPod, err := clientset.CoreV1().Pods(pod.Namespace).Create(context.TODO(), &pod, metav1.CreateOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, createdPod)
}

/**
 * 根据Namespace和Pod名称删除Pod
 */
func deletePod(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	err := clientset.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

/**
 * 获取所有Pod
 */
func getPods(c *gin.Context) {
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 定义结构体用于存储简化后的Pod信息
	type PodInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Status    string   `json:"status"`
		PodIP     string   `json:"podIP"`
		Images    []string `json:"images"`
	}

	var podInfos []PodInfo
	for _, pod := range pods.Items {
		var images []string
		for _, container := range pod.Spec.Containers {
			images = append(images, container.Image)
		}
		if pod.Namespace == "kube-system" || pod.Namespace == "kube-public" ||
			pod.Namespace == "kube-node-lease" || pod.Namespace == "kubernetes-dashboard" {
			continue
		}
		podInfos = append(podInfos, PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Status.Phase),
			PodIP:     pod.Status.PodIP,
			Images:    images,
		})
	}

	c.JSON(http.StatusOK, podInfos)
}

/**
 * 根据JSON创建Deployment
 */
func createDeployment(c *gin.Context) {
	var deployment appsv1.Deployment
	//if err := c.ShouldBindJSON(&deployment); err != nil {
	//	if err := c.ShouldBindYAML(&deployment); err != nil {
	//		if err := c.ShouldBindBodyWithJSON(&deployment); err != nil {
	//			if err := c.ShouldBindBodyWithYAML(&deployment); err != nil {
	//				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	//				return
	//			}
	//		}
	//	}
	//}

	contentType := c.Request.Header.Get("Content-Type")
	switch contentType {
	case "application/json":
		if err := c.ShouldBindJSON(&deployment); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	case "application/x-yaml":
		if err := c.ShouldBindBodyWith(&deployment, binding.YAML); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	default:
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Unsupported content type"})
		return
	}

	if deployment.Namespace == "" {
		deployment.Namespace = "default"
	}

	createdDeployment, err := clientset.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), &deployment, metav1.CreateOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, createdDeployment)
}

/**
 * 根据Namespace和Deployment名称删除Deployment
 */
func deleteDeployment(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	err := clientset.AppsV1().Deployments(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

/**
 * 获取所有Deployment
 */
func getDeployments(c *gin.Context) {
	deployments, err := clientset.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 定义结构体用于存储简化后的Deployment信息
	type DeploymentInfo struct {
		Name              string   `json:"name"`
		Namespace         string   `json:"namespace"`
		Replicas          int32    `json:"replicas"`
		AvailableReplicas int32    `json:"availableReplicas"`
		Images            []string `json:"images"`
	}

	var deploymentInfos []DeploymentInfo
	for _, deployment := range deployments.Items {
		var images []string
		for _, container := range deployment.Spec.Template.Spec.Containers {
			images = append(images, container.Image)
		}
		if deployment.Namespace == "kube-system" || deployment.Namespace == "kube-public" ||
			deployment.Namespace == "kube-node-lease" || deployment.Namespace == "kubernetes-dashboard" {
			continue
		}
		deploymentInfos = append(deploymentInfos, DeploymentInfo{
			Name:              deployment.Name,
			Namespace:         deployment.Namespace,
			Replicas:          *deployment.Spec.Replicas,
			AvailableReplicas: deployment.Status.AvailableReplicas,
			Images:            images,
		})
	}

	c.JSON(http.StatusOK, deploymentInfos)
}

/**
 * 根据JSON创建Service
 */
func createService(c *gin.Context) {
	var service v1.Service
	//if err := c.ShouldBindJSON(&service); err != nil {
	//	if err := c.ShouldBindYAML(&service); err != nil {
	//		if err := c.ShouldBindBodyWithJSON(&service); err != nil {
	//			if err := c.ShouldBindBodyWithYAML(&service); err != nil {
	//				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	//				return
	//			}
	//		}
	//	}
	//}

	contentType := c.Request.Header.Get("Content-Type")
	switch contentType {
	case "application/json":
		if err := c.ShouldBindJSON(&service); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	case "application/x-yaml":
		if err := c.ShouldBindBodyWith(&service, binding.YAML); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	default:
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Unsupported content type"})
		return
	}

	if service.Namespace == "" {
		service.Namespace = "default"

	}

	createdService, err := clientset.CoreV1().Services(service.Namespace).Create(context.TODO(), &service, metav1.CreateOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, createdService)
}

/**
 * 根据Namespace和Service名称删除Service
 */
func deleteService(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	err := clientset.CoreV1().Services(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

/**
 * 获取所有Service
 */
func getServices(c *gin.Context) {
	services, err := clientset.CoreV1().Services("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 定义结构体用于存储简化后的Service信息
	type ServiceInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Type      string   `json:"type"`
		ClusterIP string   `json:"clusterIP"`
		Ports     []string `json:"ports"`
	}

	var serviceInfos []ServiceInfo
	for _, service := range services.Items {
		var ports []string
		for _, port := range service.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
		}
		if service.Namespace == "kube-system" || service.Namespace == "kube-public" ||
			service.Namespace == "kube-node-lease" || service.Namespace == "kubernetes-dashboard" {
			continue
		}
		serviceInfos = append(serviceInfos, ServiceInfo{
			Name:      service.Name,
			Namespace: service.Namespace,
			Type:      string(service.Spec.Type),
			ClusterIP: service.Spec.ClusterIP,
			Ports:     ports,
		})
	}

	c.JSON(http.StatusOK, serviceInfos)
}

/**
 * 根据JSON创建ConfigMap
 */
func createConfigMap(c *gin.Context) {
	var configMap v1.ConfigMap
	//if err := c.ShouldBindJSON(&configMap); err != nil {
	//	if err := c.ShouldBindYAML(&configMap); err != nil {
	//		if err := c.ShouldBindBodyWithJSON(&configMap); err != nil {
	//			if err := c.ShouldBindBodyWithYAML(&configMap); err != nil {
	//				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	//				return
	//			}
	//		}
	//	}
	//}

	contentType := c.Request.Header.Get("Content-Type")
	switch contentType {
	case "application/json":
		if err := c.ShouldBindJSON(&configMap); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	case "application/x-yaml":
		if err := c.ShouldBindBodyWith(&configMap, binding.YAML); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	default:
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Unsupported content type"})
		return
	}

	if configMap.Namespace == "" {
		configMap.Namespace = "default"
	}

	createdConfigMap, err := clientset.CoreV1().ConfigMaps(configMap.Namespace).Create(context.TODO(), &configMap, metav1.CreateOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, createdConfigMap)
}

/**
 * 根据Namespace和ConfigMap名称删除ConfigMap
 */
func deleteConfigMap(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")

	err := clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

/**
 * 获取所有ConfigMap
 */
func getConfigMaps(c *gin.Context) {
	configMaps, err := clientset.CoreV1().ConfigMaps("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 定义结构体用于存储简化后的ConfigMap信息
	type ConfigMapInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Keys      []string `json:"keys"`
	}

	var configMapInfos []ConfigMapInfo
	for _, configMap := range configMaps.Items {
		var keys []string
		for key := range configMap.Data {
			keys = append(keys, key)
		}
		if configMap.Namespace == "kube-system" || configMap.Namespace == "kube-public" ||
			configMap.Namespace == "kube-node-lease" || configMap.Namespace == "kubernetes-dashboard" {
			continue
		}
		configMapInfos = append(configMapInfos, ConfigMapInfo{
			Name:      configMap.Name,
			Namespace: configMap.Namespace,
			Keys:      keys,
		})
	}

	c.JSON(http.StatusOK, configMapInfos)
}

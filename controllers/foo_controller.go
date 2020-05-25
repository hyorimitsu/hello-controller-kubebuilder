/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	samplecontrollerv1alpha1 "github.com/hyorimitsu/sample-controller-kubebuilder/api/v1alpha1"
)

// FooReconciler reconciles a Foo object
type FooReconciler struct {
	// api-server にアクセスするためのクライアント
	client.Client
	// ロガー
	Log logr.Logger
	//  runtime.Object を GVK や GVR に解決する
	Scheme *runtime.Scheme
	// Event を記録するための Recorder
	Recorder record.EventRecorder
}

// Reconcile を行うために、Foo Object の操作権限を付与
// CRUD 操作を許可
// Status Subresource への更新操作を許可
// Deployment への CRUD 操作を許可
// Event の作成操作を許可
// Reconcil関数 は、Foo Object に Event が発⽣し、Request が WorkQueue に⼊ったタイミングで呼ばれる

// +kubebuilder:rbac:groups=samplecontroller.k8s.io,resources=foos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=samplecontroller.k8s.io,resources=foos/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *FooReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("foo", req.NamespacedName)

	// Foo Object を in-memory-cache から取得
	var foo samplecontrollerv1alpha1.Foo
	log.Info("fetching Foo Resource")
	if err := r.Get(ctx, req.NamespacedName, &foo); err != nil {
		log.Error(err, "unable to fetch Foo")
		// Object が見つからない場合は再度 Reconcil しても意味がないので、
		// NotFoundError はスキップする
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 古い Deployment が存在したら削除する
	// deploymentNameが更新された場合に、古いdeploymentNameで作成されたObjectが残ってしまうため
	if err := r.cleanupOwnedResources(ctx, log, &foo); err != nil {
		log.Error(err, "failed to clean up old Deployment resources for this Foo")
		return ctrl.Result{}, err
	}

	// Deployment 作成
	deploymentName := foo.Spec.DeploymentName
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: req.Namespace,
		},
	}

	// Deployment が存在しない -> 作成
	// Deployment が存在する　 -> 更新
	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		// Create や Update が実行される前に呼ばれる

		// replicas を設定
		replicas := int32(1)
		if foo.Spec.Replicas != nil {
			replicas = *foo.Spec.Replicas
		}
		deploy.Spec.Replicas = &replicas

		// label を設定
		labels := map[string]string{
			"app":        "nginx",
			"controller": req.Name,
		}
		if deploy.Spec.Selector == nil {
			deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		}
		if deploy.Spec.Template.ObjectMeta.Labels == nil {
			deploy.Spec.Template.ObjectMeta.Labels = labels
		}

		// container を設定
		containers := []corev1.Container{
			{
				Name:  "nginx",
				Image: "nginx:latest",
			},
		}
		if deploy.Spec.Template.Spec.Containers == nil {
			deploy.Spec.Template.Spec.Containers = containers
		}

		// OwnerReference を設定
		if err := ctrl.SetControllerReference(&foo, deploy, r.Scheme); err != nil {
			log.Error(err, "unable to set ownerReference from Foo to Deployment")
			return err
		}
		return nil

	}); err != nil {
		log.Error(err, "unable to ensure deployment is correct")
		return ctrl.Result{}, err

	}

	// Deployment を in-memory-cache から取得
	var deployment appsv1.Deployment
	var deploymentNamespacedName = client.ObjectKey{Namespace: req.Namespace, Name: foo.Spec.DeploymentName}
	if err := r.Get(ctx, deploymentNamespacedName, &deployment); err != nil {
		log.Error(err, "unable to fetch Deployment")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// FooStatus を更新
	availableReplicas := deployment.Status.AvailableReplicas
	if availableReplicas == foo.Status.AvailableReplicas {
		return ctrl.Result{}, nil
	}
	foo.Status.AvailableReplicas = availableReplicas

	if err := r.Status().Update(ctx, &foo); err != nil {
		log.Error(err, "unable to update Foo status")
		return ctrl.Result{}, err
	}

	// Deployment の Update Event を記録
	r.Recorder.Eventf(&foo, corev1.EventTypeNormal, "Updated", "Update foo.status.AvailableReplicas: %d", foo.Status.AvailableReplicas)

	return ctrl.Result{}, nil
}

func (r *FooReconciler) cleanupOwnedResources(ctx context.Context, log logr.Logger, foo *samplecontrollerv1alpha1.Foo) error {
	log.Info("finding existing Deployments for Foo resource")

	// Foo Object が管理している Deployment 一覧を取得（NameSpace と Index で絞り込み）
	var deployments appsv1.DeploymentList
	if err := r.List(ctx, &deployments, client.InNamespace(foo.Namespace), client.MatchingFields(map[string]string{deploymentOwnerKey: foo.Name})); err != nil {
		return err
	}

	for _, deployment := range deployments.Items {
		// deployment の Name が Foo Object の DeploymentName と一致する場合はスキップ
		if deployment.Name == foo.Spec.DeploymentName {
			continue
		}

		// 一致しない場合は古い Object なので削除
		if err := r.Delete(ctx, &deployment); err != nil {
			log.Error(err, "failed to delete Deployment resource")
			return err
		}

		// Deployment の Delete Event を記録
		log.Info("delete deployment resource: " + deployment.Name)
		r.Recorder.Eventf(foo, corev1.EventTypeNormal, "Deleted", "Deleted deployment %q", deployment.Name)
	}

	return nil
}

var (
	deploymentOwnerKey = ".metadata.controller"
	apiGVStr           = samplecontrollerv1alpha1.GroupVersion.String()
)

// Controller Manager のセットアップ
func (r *FooReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Deployment に deploymentOwnerKey（.metadata.controller）という Index Field を追加
	if err := mgr.GetFieldIndexer().IndexField(&appsv1.Deployment{}, deploymentOwnerKey, func(rawObj runtime.Object) []string {
		deployment := rawObj.(*appsv1.Deployment)
		// Deployment の OwnerReference を取得
		owner := metav1.GetControllerOf(deployment)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != apiGVStr || owner.Kind != "Foo" {
			return nil
		}

		// Owner が Foo の場合だけ、owner.Name という Index を Deployment Object に追加
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	// Foo の監視対象として Deployment を Owns に追加
	return ctrl.NewControllerManagedBy(mgr).
		For(&samplecontrollerv1alpha1.Foo{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

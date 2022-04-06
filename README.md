# hello-controller-kubebuilder
Deployment の上位 Resource として Foo を作成

# プロジェクト作成
$ kubebuilder init --domain k8s.io

# 依存解決
$ go mod download

# テンプレート作成
$ kubebuilder create api --group samplecontroller --version v1alpha1 --kind Foo

# API Object を定義
/api/v1alpha1/foo_types.go を編集

# Reconcile を実装
/controller/foo_controller.go を編集

# main関数 を修正
main.go を修正

# CRDマニフェスト & apply
$ make install

# 動作確認
$ make run
（別コンソールを開く）
$ kubectl apply -f config/samples/samplecontroller_v1alpha1_foo.yaml
$ kubectl get foo
$ kubectl get deploy
（元のコンソールを見るとログが出ている）

# 後片付け
$ kubectl delete -f config/samples/samplecontroller_v1alpha1_foo.yaml

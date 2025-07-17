# Terraform Configuration for Cloud Run Slack Bot

この設定は、Multi-Project Cloud Run Slack BotをデプロイするためのTerraform構成です。権限を最小限に分離し、各プロジェクトで必要な設定のみを適用します。

## 📁 ディレクトリ構成

```
terraform/
├── host-project/          # Botをデプロイするプロジェクト用
│   ├── main.tf
│   └── terraform.tfvars.example
├── monitored-project/     # 監視対象プロジェクト用（各プロジェクトで実行）
│   ├── main.tf
│   └── terraform.tfvars.example
├── multi-project/         # 統合設定（オプション）
└── README.md
```

## 🔐 必要な権限の整理

### 1. Host Project（Botデプロイ先）
- **目的**: Cloud Run Slack Botのデプロイと実行
- **権限**:
  - `roles/run.invoker`: Cloud Run service invocation
  - `roles/secretmanager.secretAccessor`: Slack secrets access
  - `roles/pubsub.subscriber`: Pub/Sub message reception
  - `roles/logging.logWriter`: Log writing

### 2. Monitored Project（監視対象）
- **目的**: Slack channelに対応するprojectの監視とaudit log送信
- **権限**:
  - `roles/run.viewer`: Cloud Run resource access
  - `roles/monitoring.viewer`: Metrics access
  - `roles/logging.viewer`: Log access
  - `roles/pubsub.publisher`: Audit log publishing（Log Sink用）

## 🚀 デプロイ手順

### Step 1: Host Projectのデプロイ

```bash
# 1. Host projectディレクトリに移動
cd terraform/host-project

# 2. 設定ファイルをコピーして編集
cp terraform.tfvars.example terraform.tfvars
vi terraform.tfvars

# 3. Terraform初期化・適用
terraform init
terraform plan
terraform apply

# 4. 出力値を確認（監視対象プロジェクトで使用）
terraform output service_account_email
terraform output audit_webhook_url
```

### Step 2: 各Monitored Projectのデプロイ

各監視対象プロジェクトで以下を実行：

```bash
# 1. Monitored projectディレクトリに移動
cd terraform/monitored-project

# 2. 設定ファイルをコピーして編集
cp terraform.tfvars.example terraform.tfvars
vi terraform.tfvars

# 3. Host projectからの出力値を設定
# ⚠️ 重要: 正しいホストプロジェクトのサービスアカウントを指定してください
# bot_service_account_email = "cloud-run-slack-bot@HOST-PROJECT-ID.iam.gserviceaccount.com"
# audit_webhook_url = "https://your-bot-url.run.app/cloudrun/events"

# 4. Terraform初期化・適用
terraform init
terraform plan
terraform apply
```

### ⚠️ 重要なトラブルシューティング

もし以下のような権限エラーが出る場合：
```
Error 403: Permission 'run.services.list' denied on resource 'projects/PROJECT/locations/REGION/services'
```

**原因**: 間違ったサービスアカウントに権限が付与されている可能性があります。

**確認方法**:
```bash
# 1. 現在の権限を確認
gcloud projects get-iam-policy MONITORED-PROJECT-ID --flatten="bindings[].members" --format="table(bindings.role, bindings.members)" | grep cloud-run-slack-bot

# 2. 正しいホストプロジェクトのサービスアカウントを確認
cd terraform/host-project
terraform output service_account_email

# 3. 権限が正しくない場合は、terraform.tfvarsを修正して再適用
vi terraform.tfvars  # bot_service_account_emailを正しい値に修正
terraform plan
terraform apply
```

## 📝 設定例

### Host Project設定

```hcl
# terraform/host-project/terraform.tfvars
host_project_id = "my-bot-host-project"
slack_bot_token = "xoxb-your-bot-token"
slack_signing_secret = "your-signing-secret"

projects_config = <<EOF
[
  {
    "id": "web-frontend-project",
    "region": "us-central1",
    "defaultChannel": "web-alerts",
    "serviceChannels": {
      "web-app": "web-team",
      "api-gateway": "api-team"
    }
  },
  {
    "id": "data-pipeline-project",
    "region": "us-east1",
    "defaultChannel": "data-alerts",
    "serviceChannels": {
      "etl-job": "data-team"
    }
  }
]
EOF
```

### Monitored Project設定

```hcl
# terraform/monitored-project/terraform.tfvars（各プロジェクトで）
project_id = "web-frontend-project"
region = "us-central1"
bot_service_account_email = "cloud-run-slack-bot@my-bot-host-project.iam.gserviceaccount.com"
audit_webhook_url = "https://cloud-run-slack-bot-xxxxx-uc.a.run.app/cloudrun/events"
```

## 🔧 作成されるリソース

### Host Project
- **Cloud Run Service**: Slack Bot本体
- **Service Account**: クロスプロジェクト用
- **Secret Manager**: Slack認証情報
- **IAM Bindings**: Host project内権限

### Monitored Project（各プロジェクト）
- **IAM Bindings**: Bot用読み取り権限
- **Pub/Sub Topic**: 監査ログ用
- **Pub/Sub Subscription**: Webhook push用
- **Logging Sink**: Cloud Run監査ログ
- **IAM Bindings**: Log Sink用Publisher権限

## 🌐 Slack App設定

Host projectデプロイ完了後：

1. **Events API URL**: `terraform output webhook_url`
2. **Interactive Components URL**: `terraform output interaction_url`

## 🔄 プロジェクト追加・削除

### 新しいプロジェクトを監視対象に追加
1. Host projectの`projects_config`に新プロジェクト情報を追加
2. Host projectで`terraform apply`
3. 新プロジェクトでmonitored-project設定を実行

### プロジェクトを監視対象から削除
1. 該当プロジェクトで`terraform destroy`
2. Host projectの`projects_config`から削除
3. Host projectで`terraform apply`

## 🧹 完全クリーンアップ

```bash
# 1. 各monitored projectでリソース削除
cd terraform/monitored-project
terraform destroy

# 2. Host projectでリソース削除
cd terraform/host-project
terraform destroy
```

## 💡 メリット

1. **最小権限の原則**: 各プロジェクトに必要最小限の権限のみ付与
2. **分離された管理**: プロジェクトごとに独立してTerraform管理
3. **段階的導入**: 既存プロジェクトに段階的に監視を追加可能
4. **セキュリティ**: Cross-project権限を明示的に管理
5. **スケーラビリティ**: プロジェクト数に関係なく同じパターンで拡張可能

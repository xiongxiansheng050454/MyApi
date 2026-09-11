# 一键创建 Issues + 按模块创建并合并 PR
# 前置：winget install GitHub.cli; gh auth login
# 用法：powershell -NoProfile -ExecutionPolicy Bypass -File scripts/bootstrap-github.ps1
$ErrorActionPreference = 'Stop'

if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
    Write-Error "未检测到 gh，请先安装：winget install GitHub.cli 然后 gh auth login"
    exit 1
}

function New-Issue([string]$title, [string]$body) {
    $url = gh issue create --title $title --body $body
    return ($url -split '/')[-1]
}

$reqBody = '## 目标
统一 LLM 接入网关：下游只持网关 Key，网关转发上游并做鉴权/限流/计费/统计。

## 需求
1. 动态增删多个独立上游渠道
2. 下游只见网关 Key
3. 同模型多渠道负载均衡
4. 下游限速与限额
5. 上游异常熔断并切换
6. 以下游（用户）为聚合单位统计 token/次数/费用

详见仓库根目录 需求.txt。'

$archBody = '## 模块
1. API 层（OpenAI Chat Completions + 网关 Key 解析）
2. 下游管理（用户/Key，Key 仅存哈希）
3. 限速与配额（自研 Redis 滑动窗口 + 并发）
4. 模型路由（优先级/权重/粘性/熔断/健康）
5. 上游渠道管理（加密存储 Key、模型映射、健康检查）
6. 用量统计与费用计量（usage_logs -> Redis 增量 -> 日汇总）

## 技术栈
gin / slog / PostgreSQL / gorm / Redis / gobreaker；限流为自研滑动窗口。

详见 模块划分.txt、技术栈.txt。'

$dbBody = '用户、Key、渠道、渠道模型、定价、限流规则、用量日志、日汇总、余额、资金流水。详见 数据库表设计.txt。'

Write-Host "== 创建需求/架构/数据库 Issues =="
$iReq = New-Issue "[需求] LLM API 网关" $reqBody
$iArch = New-Issue "[架构] 模块划分与技术栈" $archBody
$iDB = New-Issue "[数据库] 表结构设计" $dbBody

Write-Host "== 创建模块 Issues =="
$mods = @(
    @{ title = "[模块] M1 项目骨架"; body = "工程骨架：config/logger/store/model/service/api/router、Dockerfile、compose。" },
    @{ title = "[模块] M2 上游+下游渠道管理"; body = "渠道 CRUD/模型映射/连通性/远端模型；下游用户与 Key（仅存哈希）。" },
    @{ title = "[模块] M3 限速配额"; body = "自研 Redis 滑动窗口 + 并发计数 + 规则 CRUD。" },
    @{ title = "[模块] M4 模型路由"; body = "优先级分组/权重无放回/组尽降级 + user_id 粘性（Redis 亲和）+ 熔断。" },
    @{ title = "[模块] M5 定价管理"; body = "渠道x模型单价 CRUD 与 upsert。" },
    @{ title = "[模块] M6 用量统计与费用计量"; body = "usage_logs 落库 + Redis 增量 + RENAME 快照定时刷入 user_daily_stats + 查询接口。" },
    @{ title = "[模块] M7 预冻结与结算扣费"; body = "Redis 预扣（Lua/gen/DCL）+ DB 权威结算 + 分词器估算 + 三场景计费。" },
    @{ title = "[模块] M8 模型列表与 Key 使用时间"; body = "GET /v1/models；Key 列表派生 last_used_at。" },
    @{ title = "[模块] M9 渠道余额前置筛选"; body = "渠道本地余额、结算扣减、低水位剔除、管理端调整。" },
    @{ title = "[模块] M10 流式重试与止损"; body = "同渠道重试、空闲超时、下游断开取消上游、已输出后终止。" },
    @{ title = "[模块] M11 Dashboard"; body = "API 网关仪表盘前端。" }
)
$modIssue = @{}
foreach ($m in $mods) { $modIssue[$m.title] = New-Issue $m.title $m.body }

Write-Host "== 按序创建并合并模块 PR =="
$prs = @(
    @{ head = "feature/01-skeleton";        title = "feat: 项目骨架";                          issue = $modIssue["[模块] M1 项目骨架"] },
    @{ head = "feature/02-channel-mgmt";    title = "feat: 上游渠道与下游 Key 管理";            issue = $modIssue["[模块] M2 上游+下游渠道管理"] },
    @{ head = "feature/03-rate-limit";      title = "feat: 限速与配额";                        issue = $modIssue["[模块] M3 限速配额"] },
    @{ head = "feature/04-routing";         title = "feat: 模型路由（优先级/权重/粘性/熔断）";  issue = $modIssue["[模块] M4 模型路由"] },
    @{ head = "feature/05-pricing";         title = "feat: 渠道x模型定价";                     issue = $modIssue["[模块] M5 定价管理"] },
    @{ head = "feature/06-stats";           title = "feat: 用量统计与费用计量";                issue = $modIssue["[模块] M6 用量统计与费用计量"] },
    @{ head = "feature/07-billing";         title = "feat: 预冻结与结算扣费";                  issue = $modIssue["[模块] M7 预冻结与结算扣费"] },
    @{ head = "feature/08-models-keys";     title = "feat: 模型列表与 Key 使用时间";            issue = $modIssue["[模块] M8 模型列表与 Key 使用时间"] },
    @{ head = "feature/09-channel-balance"; title = "feat: 渠道余额前置筛选";                  issue = $modIssue["[模块] M9 渠道余额前置筛选"] },
    @{ head = "feature/10-streaming";       title = "feat: 流式重试与止损";                    issue = $modIssue["[模块] M10 流式重试与止损"] },
    @{ head = "feature/11-dashboard";       title = "feat: API 网关仪表盘";                    issue = $modIssue["[模块] M11 Dashboard"] }
)
foreach ($p in $prs) {
    Write-Host ("创建 PR: " + $p.head)
    $body = "Closes #" + $p.issue + "`n`n关联 需求 #" + $iReq + " / 架构 #" + $iArch + " / 数据库 #" + $iDB
    gh pr create --base master --head $p.head --title $p.title --body $body | Out-Null
    $n = gh pr list --head $p.head --state open --json number -q ".[0].number"
    gh pr merge $n --merge | Out-Null
}

Write-Host "== 清理旧的 dashboard 分支（PR #2 已作废）=="
git push origin --delete feature/api-gateway-dashboard 2>$null

Write-Host "完成。"

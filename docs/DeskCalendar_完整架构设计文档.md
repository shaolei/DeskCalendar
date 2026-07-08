# DeskCalendar 完整架构设计文档

版本0.1 日期:2026-07-06

# 第1章 项目概述

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第2章 总体架构

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第3章 技术选型

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第4章 目录结构

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第5章 Shell设计

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第6章 Feature设计

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第7章 Platform设计

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第8章 State设计

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第9章 数据存储

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第10章 主题系统

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第11章 插件系统

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第12章 UI设计

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第13章 Windows集成

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第14章 开发规范

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第15章 测试策略

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 第16章 开发路线图

## 设计目标

-   高内聚、低耦合
-   Shell + Feature 架构
-   响应式状态驱动
-   Repository 模式
-   平台能力隔离

## 推荐实践

``` text
shell -> feature -> service -> repository -> storage
             ^
           state(signal)
```

## 内容

Shell 负责生命周期与窗口；Feature 负责业务；Platform 封装 Windows
API（WorkerW、Mica、通知、自启动、DPI）；State 保存全局状态；Storage
管理 SQLite/JSON；Theme 与 Plugin 可独立扩展。

## 示例接口

``` go
type Plugin interface {
    Name() string
    Start() error
    Stop() error
}
```

# 附录

``` text
DeskCalendar
├── cmd
├── internal
│   ├── shell
│   ├── features
│   ├── platform/windows
│   ├── services
│   ├── state
│   ├── storage
│   ├── theme
│   └── plugins
├── assets
├── configs
└── docs
```

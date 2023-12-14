# plan

## wip
- 本地离线文件 + 初始化
- 变更缓存策略
- 测试win/mac的DB初始化情况

## short-term
- 多source直接嵌套进列表
- 加上变形(addtional pattern
- 处理查询时候的大小写，避免过度存储
- 记录pid/port，先检查这两个
- server悄悄下载单词文件就可以了
- 预留一个接口调用，获取重要信息
- 用代理爬词库
- 长句查询另外缓存
- use hash as cache file's name for long query
- json储存所有查过的单词最后访问时间

## Long-term
- 引入stardict源
- 增加服务端
- 更新自动提醒
- 自动更新ua数据
- 加入词库设置，供选择词库大小

## low priority
- source数据 枚举
- 刷数据，去掉音标[]
- server增加信号处理，做一些善后处理like删除文件
- not found结果写入硬盘，查询时候两个协程同时查两种数据
- 检测配置保存时间变化的基础上再加上内容判断？
- not found list记录查询时间，超时删除
- not found和索引都加入服务端缓存，benchmark比较直接查本地和tcp通信的速度
- cli替换为cobra
- 自定义json mashal的encoder

# BUG

## Risk
- 实际文件名 不改的时候的process_name

# Others

## 写入release介绍

架构选择：
- 常用的64位Intel/AMD选择amd64
- 32位选择386
- Mac m1/2/3芯片选择arm64架构
- 注意区分amd64和arm64

如果没有你所使用的平台/架构，请提交issue反馈

如果下载受阻，请前往gitee备份页面

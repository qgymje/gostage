# GoStage: 一个简洁的Go任务框架

### 设计理念

GoStage的设计理念是来自于Unix的管道，其发明人Doug Mcllroy曾经说过:
> 让每个程序就做好一件事情。

GoStage鼓励将任务分割，每一步只做一件事情，上一步的输出就是下一步的输入，就像Unix管道一样。
与此同时，在Go语言并发特性的支持下，在一个GoStage实例里，所有的Worker都是并发执行的。

### 基本概念

在GoStage里，任意一个Stage，需要实现```Worker```接口, 定义如下：

```go
type Worker interface{
    HandleEvent(interface{}) (interface{}, error)
}
```

与此同时，同样提供了函数类型的实现方式：

```go
// WorkHandler is a handy function type that implements Worker
type WorkHandler func(interface{}) (interface{}, error)
```

每个Worker可以扮演的角色是Producer(生产者), ProducerConsumer(中间者), Consumer(消费者)。
其中Producer的HandleEvent的输入参数是nil，其它角色的输入参数是上一步输出。

### 将Worker联系起来
GoStage通过```Config```对象将```Worker```关联起来：

```go
// w1, w2, w3都是实现了Worker接口的对象

cfg := []*gostage.Config{
    {
        Worker: w1,
    },
    {
        Worker: w2,
        SubscribeTo: w1,
    },
    {
        Worker: w3,
        SubscribeTo: w2,
    },
}
```

以上示例，表示w1为Producer，产生数据，w2为ProducerConsumer，w3为Consumer，具体可以参看examples.


### Worker的配置

```gostage.Config```的具体内容如下：

```go
type Config struct {
    Name string
	Size int
	Worker Worker
	SubscribeTo Worker
	Restart int
}
```

 * ```Name```表示一个Worker的名字，不是必填，体现在Logger中表示是log是哪个Worker生产的, 默认是Worker实例的名字；
 * ```Size```表示这个Worker需要启动多少个Goroutine去并发地执行任务。默认为1，如果超过1个，则Worker需要额外定义Create()方法，签名如下：
    ```go
    Create() gostage.Worker
    ```
* Create()一个返回Worker实例，可以根据需要创建与原Worker独立的数据或者共享的数据;

* ```Worker```表示Worker实例;
* ```SubscribeTo```表示这个Worker需要从哪个Worker里获取数据，如果是Producer可省略，除此之外是必填，否则数据流不起来；
* ```Restart```表示每个运行在独立goroutine里的Worker可以因为异常重启多少次，默认为1次，可通过全局```DefaultRestart```改变所有Worker的重启数次


### 全局配置
* 如果一个Worker定义了```Close()```方法，则GoStage会在程序退出的时候，调用Close()方法，用于关闭一些资源。
* 如果Producer返回的error是```ErrNoData```，则GoStage会积累一定数次之后，将停止一定时间再调用Producer, 可通过```NoDataCount```修改积累次数，```NoDataCountSleep```控制暂停时间
* 如果Producer返回的error是```ErrQuit```, 则GoStage将会退出



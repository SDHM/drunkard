package sd

/**
 *	服务发现注册者接口
 *
 *
 */

type IRegister interface {
	Register() error
	UnRegister() error
}

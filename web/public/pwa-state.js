export function memberSetup({ios,installed,supported,permission,subscribed,verificationFailed=false,subscriptionMissing=false}) {
 if(ios&&!installed)return 'install-first'
 if(!supported)return 'unsupported'
 if(permission==='denied')return 'blocked'
 if(permission!=='granted')return 'enable'
 if(subscriptionMissing)return 'reconnect'
 if(verificationFailed)return 'retry'
 if(!subscribed)return 'enable'
 return installed?'ready':'install'
}

export function memberClan(value) {
 return ['1','2','3'].includes(value)?value:''
}

export function memberAppURL(clan) {
 const id=memberClan(clan)
 return id?'/app?clan='+id:'/app'
}

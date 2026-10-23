export function memberSetup({ios,installed,supported,permission,subscribed}) {
 if(ios&&!installed)return 'install-first'
 if(!supported)return 'unsupported'
 if(permission==='denied')return 'blocked'
 if(permission!=='granted'||!subscribed)return 'enable'
 return installed?'ready':'install'
}

// Use fixed values rather than carrying DOM, query-string or stored text into
// navigation URLs. The member app supports exactly these two clans.
export function memberClan(value) {
 if(value==='1')return '1'
 if(value==='2')return '2'
 return ''
}

export function memberAppURL(clan) {
 if(clan==='1')return '/app?clan=1'
 if(clan==='2')return '/app?clan=2'
 return '/app'
}

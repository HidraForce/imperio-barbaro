export function memberSetup({ios,installed,supported,permission,subscribed}) {
 if(ios&&!installed)return 'install-first'
 if(!supported)return 'unsupported'
 if(permission==='denied')return 'blocked'
 if(permission!=='granted'||!subscribed)return 'enable'
 return installed?'ready':'install'
}

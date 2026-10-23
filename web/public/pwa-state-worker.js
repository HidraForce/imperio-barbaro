// The push payload is never allowed to navigate outside the member application.
function safePushURL(value){try{const url=new URL(value||'/app',self.location.origin);if(url.origin===self.location.origin&&url.pathname==='/app')return url.href}catch{}return self.location.origin+'/app'}

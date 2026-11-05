'use strict';
const boot=document.createElement('script');
boot.src=window.QUIET_NATIVE?'native-app.js':'app.js';
document.body.append(boot);

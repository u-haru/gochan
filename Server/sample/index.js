function parsedat(key,dat,shownum){
	const addres = (res, i) => {
		let el = `<dt class="header" id="${key}_${i}">${i}<span class="name">${res.from}</span>${res.mail ? `<span class="mail">${res.mail}</span>` : ""}${res.date_id}</dt>`
		el += `<dd class="res">${res.message}</dd>`
		return el
	}
	let datsplit = dat.split("\n")
	let thread = { "title": "", "res": [] }
	datsplit.forEach(res => {
		if (res != "") {
			let tmp = res.split("<>")
			tmp[3] = tmp[3].replace(/&gt;&gt;([0-9]+)(?![-\d])/i, `<a href="#${key}_$1">&gt;&gt;$1</a>`)
			tmp[3] = tmp[3].replace(/&gt;&gt;([0-9]+)\-([0-9]+)/i, `<a href="#${key}_$1">&gt;&gt;$1</a>-<a href="#${key}_$2">&gt;&gt;$2</a>`)
			tmp[3] = tmp[3].replace(/([a-z]+:\/\/[!-z]*)/i, `<a href="$1">$1</a>`)
			thread.res.push({ "from": tmp[0], "mail": tmp[1], "date_id": tmp[2], "message": tmp[3] })
			if (tmp[4]) {
				thread.title = tmp[4]
			}
		}
	});

	let dl = `<dt class="title">${thread.title}</dt>`
	if (shownum > 0 && thread.res.length > shownum) {
		dl += addres(thread.res[0], 1)
		thread.res.slice(-shownum+1).forEach((res, i) => {
			dl += addres(res, thread.res.length - shownum + 2 + i)
		})
	} else {
		thread.res.forEach((res, i) => {
			dl += addres(res, i + 1)
		})
	}

	return dl
}
function getdat(key,callback) {
	let xhr = new XMLHttpRequest();
	xhr.open('GET', "./dat/" + key + ".dat",true);
	xhr.onload = function () {
		callback(xhr.responseText)
	};
	xhr.send();
}
function postarea(bbs,key) {
	return `<div class="common">${key ? "書き込み欄" : "新規スレッド作成"}</div>
			<div style="margin: 0.5em 2em; font-size: 0.75em;">
				<form method="POST" action="/test/bbs.cgi" accept-charset="Shift-JIS">
					<input type="submit" value="${key ? "書き込み" : "新規スレッド"}"><br>
					${!key ? `<div>スレッドタイトル：<input type="text" name="subject" style="width: 24em;"></div>` : ""}
					<div>
						<div style="display:inline-block">名前：<input type="text" name="FROM" style="width: 16em;"></div>
						<div style="display:inline-block">E-mail：<input type="text" name="mail" style="width: 16em;"></div>
					</div>
					<textarea style="width: 40em; height: 10.0em; word-wrap: break-word;" rows="4" cols="12"
						name="MESSAGE"></textarea>
					<input type="hidden" name="bbs" value="${bbs}">
					${key ? `<input type="hidden" name="key" value="${key}">` : ""}
				</form>
			</div>`
}
function loadsubs(callback){
	let xhr = new XMLHttpRequest();
	xhr.open('GET', './subject.txt',true);
	xhr.onload = function () {
		callback(xhr.responseText.split("\n"))
	};
	xhr.send();
}
//https://stackoverflow.com/questions/3219758/detect-changes-in-the-dom
function observeDOM( obj, callback ){
	const MutationObserver = window.MutationObserver || window.WebKitMutationObserver;
	if( !obj || obj.nodeType !== 1 ) return; 
	if( MutationObserver ){
		let mutationObserver = new MutationObserver(callback)
		mutationObserver.observe( obj, { childList:true, subtree:true })
		return mutationObserver
	}else if( window.addEventListener ){
		obj.addEventListener('DOMNodeInserted', callback, false)
		obj.addEventListener('DOMNodeRemoved', callback, false)
	}
}
function loadIframe(elem,src){
	let xhr = new XMLHttpRequest();
	xhr.open('GET', src);
	xhr.onload = function () {
		let iframe = document.createElement('iframe');
		iframe.setAttribute("scrolling","no")
		iframe.setAttribute("style","width:100%;border:none;")
		elem.insertAdjacentElement("afterbegin",iframe)
		let doc = iframe.contentWindow.document;
		doc.open();
		doc.write(xhr.responseText);
		doc.close();
		iframe.style.height = doc.firstElementChild.clientHeight+ "px"
		observeDOM(doc.firstElementChild,()=>{
			iframe.style.height = doc.firstElementChild.clientHeight+ "px"
		})
	}
	xhr.overrideMimeType('text/html; charset=Shift_JIS')
	xhr.send();
}
function createElementFromHTML(html) {
	let template = document.createElement('template');
	html = html.trim(); // Never return a text node of whitespace as the result
	template.innerHTML = html;
	return template.content.firstElementChild;
}
export namespace battery {
	
	export class Battery {
	    name: string;
	    state: string;
	    chargePercent: number;
	    designWh: number;
	    fullWh: number;
	    healthPercent: number;
	    cycleCount: number;
	    technology: string;
	    manufacturer: string;
	    model: string;
	    serial: string;
	    verdict: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new Battery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.state = source["state"];
	        this.chargePercent = source["chargePercent"];
	        this.designWh = source["designWh"];
	        this.fullWh = source["fullWh"];
	        this.healthPercent = source["healthPercent"];
	        this.cycleCount = source["cycleCount"];
	        this.technology = source["technology"];
	        this.manufacturer = source["manufacturer"];
	        this.model = source["model"];
	        this.serial = source["serial"];
	        this.verdict = source["verdict"];
	        this.source = source["source"];
	    }
	}
	export class Report {
	    os: string;
	    batteries: Battery[];
	    onAc: boolean;
	    hasAc: boolean;
	    unsupported: string[];
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.batteries = this.convertValues(source["batteries"], Battery);
	        this.onAc = source["onAc"];
	        this.hasAc = source["hasAc"];
	        this.unsupported = source["unsupported"];
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace certdec {
	
	export class Name {
	    commonName: string;
	    organization: string[];
	    organizationalUnit: string[];
	    country: string[];
	
	    static createFrom(source: any = {}) {
	        return new Name(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commonName = source["commonName"];
	        this.organization = source["organization"];
	        this.organizationalUnit = source["organizationalUnit"];
	        this.country = source["country"];
	    }
	}
	export class Certificate {
	    index: number;
	    subject: Name;
	    issuer: Name;
	    subjectLine: string;
	    issuerLine: string;
	    serialNumber: string;
	    notBefore: string;
	    notAfter: string;
	    daysRemaining: number;
	    expired: boolean;
	    notYetValid: boolean;
	    daysUntilValid: number;
	    signatureAlgorithm: string;
	    weakSignature: boolean;
	    publicKeyAlgorithm: string;
	    publicKeyBits: number;
	    publicKeyLabel: string;
	    dnsNames: string[];
	    ipAddresses: string[];
	    emailAddresses: string[];
	    uris: string[];
	    keyUsage: string[];
	    extendedKeyUsage: string[];
	    isCa: boolean;
	    basicConstraintsValid: boolean;
	    maxPathLen: number;
	    pathLenText: string;
	    sha1Fingerprint: string;
	    sha256Fingerprint: string;
	    subjectKeyId: string;
	    authorityKeyId: string;
	    selfSigned: boolean;
	    version: number;
	    issuerInFile: number;
	    status: string;
	    statusLabel: string;
	    headline: string;
	    notes: string[];
	    pem: string;
	
	    static createFrom(source: any = {}) {
	        return new Certificate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.subject = this.convertValues(source["subject"], Name);
	        this.issuer = this.convertValues(source["issuer"], Name);
	        this.subjectLine = source["subjectLine"];
	        this.issuerLine = source["issuerLine"];
	        this.serialNumber = source["serialNumber"];
	        this.notBefore = source["notBefore"];
	        this.notAfter = source["notAfter"];
	        this.daysRemaining = source["daysRemaining"];
	        this.expired = source["expired"];
	        this.notYetValid = source["notYetValid"];
	        this.daysUntilValid = source["daysUntilValid"];
	        this.signatureAlgorithm = source["signatureAlgorithm"];
	        this.weakSignature = source["weakSignature"];
	        this.publicKeyAlgorithm = source["publicKeyAlgorithm"];
	        this.publicKeyBits = source["publicKeyBits"];
	        this.publicKeyLabel = source["publicKeyLabel"];
	        this.dnsNames = source["dnsNames"];
	        this.ipAddresses = source["ipAddresses"];
	        this.emailAddresses = source["emailAddresses"];
	        this.uris = source["uris"];
	        this.keyUsage = source["keyUsage"];
	        this.extendedKeyUsage = source["extendedKeyUsage"];
	        this.isCa = source["isCa"];
	        this.basicConstraintsValid = source["basicConstraintsValid"];
	        this.maxPathLen = source["maxPathLen"];
	        this.pathLenText = source["pathLenText"];
	        this.sha1Fingerprint = source["sha1Fingerprint"];
	        this.sha256Fingerprint = source["sha256Fingerprint"];
	        this.subjectKeyId = source["subjectKeyId"];
	        this.authorityKeyId = source["authorityKeyId"];
	        this.selfSigned = source["selfSigned"];
	        this.version = source["version"];
	        this.issuerInFile = source["issuerInFile"];
	        this.status = source["status"];
	        this.statusLabel = source["statusLabel"];
	        this.headline = source["headline"];
	        this.notes = source["notes"];
	        this.pem = source["pem"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Result {
	    certificates: Certificate[];
	    format: string;
	    source: string;
	    chainNote: string;
	    inOrder: boolean;
	    suggestedOrder: number[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.certificates = this.convertValues(source["certificates"], Certificate);
	        this.format = source["format"];
	        this.source = source["source"];
	        this.chainNote = source["chainNote"];
	        this.inOrder = source["inOrder"];
	        this.suggestedOrder = source["suggestedOrder"];
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace certgen {
	
	export class Params {
	    commonName: string;
	    sans: string[];
	    organization: string;
	    orgUnit: string;
	    country: string;
	    state: string;
	    locality: string;
	    email: string;
	    keyType: string;
	    days: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commonName = source["commonName"];
	        this.sans = source["sans"];
	        this.organization = source["organization"];
	        this.orgUnit = source["orgUnit"];
	        this.country = source["country"];
	        this.state = source["state"];
	        this.locality = source["locality"];
	        this.email = source["email"];
	        this.keyType = source["keyType"];
	        this.days = source["days"];
	    }
	}
	export class Result {
	    privateKeyPem: string;
	    certificatePem: string;
	    csrPem: string;
	    subject: string;
	    dnsNames: string[];
	    ipAddresses: string[];
	    keyLabel: string;
	    notBefore: string;
	    notAfter: string;
	    days: number;
	    serialNumber: string;
	    fingerprint: string;
	    suggestedName: string;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.privateKeyPem = source["privateKeyPem"];
	        this.certificatePem = source["certificatePem"];
	        this.csrPem = source["csrPem"];
	        this.subject = source["subject"];
	        this.dnsNames = source["dnsNames"];
	        this.ipAddresses = source["ipAddresses"];
	        this.keyLabel = source["keyLabel"];
	        this.notBefore = source["notBefore"];
	        this.notAfter = source["notAfter"];
	        this.days = source["days"];
	        this.serialNumber = source["serialNumber"];
	        this.fingerprint = source["fingerprint"];
	        this.suggestedName = source["suggestedName"];
	        this.warnings = source["warnings"];
	    }
	}

}

export namespace discover {
	
	export class Params {
	    timeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeoutMs = source["timeoutMs"];
	    }
	}

}

export namespace diskbench {
	
	export class Params {
	    path: string;
	    sizeMb: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.sizeMb = source["sizeMb"];
	    }
	}

}

export namespace diskscan {
	
	export class Params {
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	    }
	}

}

export namespace dnscmp {
	
	export class Answer {
	    server: string;
	    label: string;
	    values: string[];
	    status: string;
	    message: string;
	    queryMs: number;
	    inStep: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Answer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = source["server"];
	        this.label = source["label"];
	        this.values = source["values"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.queryMs = source["queryMs"];
	        this.inStep = source["inStep"];
	    }
	}
	export class Comparison {
	    name: string;
	    type: string;
	    answers: Answer[];
	    majority: string[];
	    majorityCount: number;
	    answered: number;
	    agree: boolean;
	    fastestLabel: string;
	    fastestMs: number;
	    level: string;
	    headline: string;
	    advice: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Comparison(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.answers = this.convertValues(source["answers"], Answer);
	        this.majority = source["majority"];
	        this.majorityCount = source["majorityCount"];
	        this.answered = source["answered"];
	        this.agree = source["agree"];
	        this.fastestLabel = source["fastestLabel"];
	        this.fastestMs = source["fastestMs"];
	        this.level = source["level"];
	        this.headline = source["headline"];
	        this.advice = source["advice"];
	        this.checkedAt = source["checkedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Params {
	    name: string;
	    type: string;
	    servers: string[];
	    timeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.servers = source["servers"];
	        this.timeoutMs = source["timeoutMs"];
	    }
	}
	export class ServerOption {
	    id: string;
	    label: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.detail = source["detail"];
	    }
	}

}

export namespace dnslook {
	
	export class Params {
	    name: string;
	    types: string[];
	    servers: string[];
	    timeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.types = source["types"];
	        this.servers = source["servers"];
	        this.timeoutMs = source["timeoutMs"];
	    }
	}
	export class ServerOption {
	    id: string;
	    label: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.detail = source["detail"];
	    }
	}

}

export namespace dupfind {
	
	export class Params {
	    path: string;
	    minBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.minBytes = source["minBytes"];
	    }
	}

}

export namespace filedrop {
	
	export class Address {
	    ip: string;
	    adapter: string;
	    primary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Address(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.adapter = source["adapter"];
	        this.primary = source["primary"];
	    }
	}
	export class Params {
	    files: string[];
	    port: number;
	    allowUpload: boolean;
	    receiveDir: string;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.port = source["port"];
	        this.allowUpload = source["allowUpload"];
	        this.receiveDir = source["receiveDir"];
	    }
	}
	export class Session {
	    token: string;
	    port: number;
	    url: string;
	    files: number;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.port = source["port"];
	        this.url = source["url"];
	        this.files = source["files"];
	    }
	}

}

export namespace firewall {
	
	export class Hint {
	    firewall: string;
	    message: string;
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new Hint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.firewall = source["firewall"];
	        this.message = source["message"];
	        this.command = source["command"];
	    }
	}

}

export namespace hashfile {
	
	export class Digests {
	    path: string;
	    name: string;
	    bytes: number;
	    md5: string;
	    sha1: string;
	    sha256: string;
	
	    static createFrom(source: any = {}) {
	        return new Digests(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.bytes = source["bytes"];
	        this.md5 = source["md5"];
	        this.sha1 = source["sha1"];
	        this.sha256 = source["sha256"];
	    }
	}
	export class Params {
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	    }
	}
	export class Verdict {
	    state: string;
	    algorithm: string;
	    expected: string;
	    actual: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Verdict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.algorithm = source["algorithm"];
	        this.expected = source["expected"];
	        this.actual = source["actual"];
	        this.message = source["message"];
	    }
	}

}

export namespace hibp {
	
	export class Result {
	    checked: boolean;
	    found: boolean;
	    count: number;
	    prefix: string;
	    compared: number;
	    level: string;
	    verdict: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.checked = source["checked"];
	        this.found = source["found"];
	        this.count = source["count"];
	        this.prefix = source["prefix"];
	        this.compared = source["compared"];
	        this.level = source["level"];
	        this.verdict = source["verdict"];
	        this.checkedAt = source["checkedAt"];
	    }
	}

}

export namespace ipscan {
	
	export class Defaults {
	    range: string;
	    adapter: string;
	    ip: string;
	    gateway: string;
	
	    static createFrom(source: any = {}) {
	        return new Defaults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.range = source["range"];
	        this.adapter = source["adapter"];
	        this.ip = source["ip"];
	        this.gateway = source["gateway"];
	    }
	}
	export class ScanParams {
	    range: string;
	    timeoutMs: number;
	    workers: number;
	    ports: number[];
	    skipTcp: boolean;
	    skipHostnames: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ScanParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.range = source["range"];
	        this.timeoutMs = source["timeoutMs"];
	        this.workers = source["workers"];
	        this.ports = source["ports"];
	        this.skipTcp = source["skipTcp"];
	        this.skipHostnames = source["skipHostnames"];
	    }
	}

}

export namespace lanspeed {
	
	export class Params {
	    port: number;
	    sizeMb: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.sizeMb = source["sizeMb"];
	    }
	}
	export class Session {
	    token: string;
	    port: number;
	    url: string;
	    sizeMb: number;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.port = source["port"];
	        this.url = source["url"];
	        this.sizeMb = source["sizeMb"];
	    }
	}

}

export namespace logview {
	
	export class Line {
	    number: number;
	    offset: number;
	    text: string;
	    level: string;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Line(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.offset = source["offset"];
	        this.text = source["text"];
	        this.level = source["level"];
	        this.truncated = source["truncated"];
	    }
	}
	export class Chunk {
	    lines: Line[];
	    start: number;
	    end: number;
	    bytes: number;
	    atStart: boolean;
	    atEnd: boolean;
	    shrank: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Chunk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lines = this.convertValues(source["lines"], Line);
	        this.start = source["start"];
	        this.end = source["end"];
	        this.bytes = source["bytes"];
	        this.atStart = source["atStart"];
	        this.atEnd = source["atEnd"];
	        this.shrank = source["shrank"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Info {
	    path: string;
	    name: string;
	    bytes: number;
	    modified: string;
	    crlf: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.bytes = source["bytes"];
	        this.modified = source["modified"];
	        this.crlf = source["crlf"];
	    }
	}
	
	export class ReadParams {
	    path: string;
	    where: string;
	    offset: number;
	    lines: number;
	
	    static createFrom(source: any = {}) {
	        return new ReadParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.where = source["where"];
	        this.offset = source["offset"];
	        this.lines = source["lines"];
	    }
	}
	export class SearchParams {
	    path: string;
	    query: string;
	    matchCase: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SearchParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.query = source["query"];
	        this.matchCase = source["matchCase"];
	    }
	}

}

export namespace maildns {
	
	export class DKIMKey {
	    selector: string;
	    record: string;
	    hasKey: boolean;
	    keyType: string;
	
	    static createFrom(source: any = {}) {
	        return new DKIMKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.selector = source["selector"];
	        this.record = source["record"];
	        this.hasKey = source["hasKey"];
	        this.keyType = source["keyType"];
	    }
	}
	export class DMARC {
	    found: boolean;
	    record: string;
	    count: number;
	    policy: string;
	    subdomain: string;
	    pct: number;
	    rua: string[];
	    ruf: string[];
	    verdict: string;
	
	    static createFrom(source: any = {}) {
	        return new DMARC(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.record = source["record"];
	        this.count = source["count"];
	        this.policy = source["policy"];
	        this.subdomain = source["subdomain"];
	        this.pct = source["pct"];
	        this.rua = source["rua"];
	        this.ruf = source["ruf"];
	        this.verdict = source["verdict"];
	    }
	}
	export class Finding {
	    level: string;
	    area: string;
	    title: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new Finding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.area = source["area"];
	        this.title = source["title"];
	        this.detail = source["detail"];
	    }
	}
	export class MXHost {
	    host: string;
	    preference: number;
	
	    static createFrom(source: any = {}) {
	        return new MXHost(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.preference = source["preference"];
	    }
	}
	export class Params {
	    domain: string;
	    selector: string;
	    server: string;
	    timeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.selector = source["selector"];
	        this.server = source["server"];
	        this.timeoutMs = source["timeoutMs"];
	    }
	}
	export class SPFTerm {
	    qualifier: string;
	    mechanism: string;
	    value: string;
	    costsLookup: boolean;
	    raw: string;
	
	    static createFrom(source: any = {}) {
	        return new SPFTerm(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.qualifier = source["qualifier"];
	        this.mechanism = source["mechanism"];
	        this.value = source["value"];
	        this.costsLookup = source["costsLookup"];
	        this.raw = source["raw"];
	    }
	}
	export class SPF {
	    found: boolean;
	    record: string;
	    count: number;
	    terms: SPFTerm[];
	    all: string;
	    redirect: string;
	    lookups: number;
	    verdict: string;
	
	    static createFrom(source: any = {}) {
	        return new SPF(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.record = source["record"];
	        this.count = source["count"];
	        this.terms = this.convertValues(source["terms"], SPFTerm);
	        this.all = source["all"];
	        this.redirect = source["redirect"];
	        this.lookups = source["lookups"];
	        this.verdict = source["verdict"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Report {
	    domain: string;
	    server: string;
	    mx: MXHost[];
	    nullMx: boolean;
	    spf: SPF;
	    dmarc: DMARC;
	    dkim: DKIMKey[];
	    selectorsTried: string[];
	    findings: Finding[];
	    level: string;
	    headline: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.server = source["server"];
	        this.mx = this.convertValues(source["mx"], MXHost);
	        this.nullMx = source["nullMx"];
	        this.spf = this.convertValues(source["spf"], SPF);
	        this.dmarc = this.convertValues(source["dmarc"], DMARC);
	        this.dkim = this.convertValues(source["dkim"], DKIMKey);
	        this.selectorsTried = source["selectorsTried"];
	        this.findings = this.convertValues(source["findings"], Finding);
	        this.level = source["level"];
	        this.headline = source["headline"];
	        this.checkedAt = source["checkedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace main {
	
	export class AppInfo {
	    version: string;
	    platform: string;
	    storeDir: string;
	    portable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.platform = source["platform"];
	        this.storeDir = source["storeDir"];
	        this.portable = source["portable"];
	    }
	}

}

export namespace netinfo {
	
	export class IPv6 {
	    ip: string;
	    prefix: number;
	    cidr: string;
	    scope: string;
	
	    static createFrom(source: any = {}) {
	        return new IPv6(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.prefix = source["prefix"];
	        this.cidr = source["cidr"];
	        this.scope = source["scope"];
	    }
	}
	export class IPv4 {
	    ip: string;
	    mask: string;
	    prefix: number;
	    cidr: string;
	    network: string;
	    broadcast: string;
	
	    static createFrom(source: any = {}) {
	        return new IPv4(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.mask = source["mask"];
	        this.prefix = source["prefix"];
	        this.cidr = source["cidr"];
	        this.network = source["network"];
	        this.broadcast = source["broadcast"];
	    }
	}
	export class Adapter {
	    name: string;
	    friendlyName: string;
	    description: string;
	    index: number;
	    up: boolean;
	    mac: string;
	    mtu: number;
	    loopback: boolean;
	    virtual: boolean;
	    primary: boolean;
	    ipv4: IPv4[];
	    ipv6: IPv6[];
	    gateway: string;
	    dns: string[];
	    dhcp: string;
	
	    static createFrom(source: any = {}) {
	        return new Adapter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.friendlyName = source["friendlyName"];
	        this.description = source["description"];
	        this.index = source["index"];
	        this.up = source["up"];
	        this.mac = source["mac"];
	        this.mtu = source["mtu"];
	        this.loopback = source["loopback"];
	        this.virtual = source["virtual"];
	        this.primary = source["primary"];
	        this.ipv4 = this.convertValues(source["ipv4"], IPv4);
	        this.ipv6 = this.convertValues(source["ipv6"], IPv6);
	        this.gateway = source["gateway"];
	        this.dns = source["dns"];
	        this.dhcp = source["dhcp"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class Report {
	    os: string;
	    hostname: string;
	    adapters: Adapter[];
	    dns: string[];
	    searchDomains: string[];
	    unsupported: string[];
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.hostname = source["hostname"];
	        this.adapters = this.convertValues(source["adapters"], Adapter);
	        this.dns = source["dns"];
	        this.searchDomains = source["searchDomains"];
	        this.unsupported = source["unsupported"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace netstat {
	
	export class Entry {
	    protocol: string;
	    address: string;
	    port: number;
	    reach: string;
	    pid: number;
	    process: string;
	    service: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protocol = source["protocol"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.reach = source["reach"];
	        this.pid = source["pid"];
	        this.process = source["process"];
	        this.service = source["service"];
	        this.source = source["source"];
	    }
	}
	export class Report {
	    os: string;
	    entries: Entry[];
	    processNames: boolean;
	    unsupported: string[];
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.entries = this.convertValues(source["entries"], Entry);
	        this.processNames = source["processNames"];
	        this.unsupported = source["unsupported"];
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace ntpcheck {
	
	export class Params {
	    servers: string[];
	    timeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.servers = source["servers"];
	        this.timeoutMs = source["timeoutMs"];
	    }
	}
	export class Server {
	    server: string;
	    address: string;
	    offsetMs: number;
	    delayMs: number;
	    stratum: number;
	    serverTime: string;
	    localTime: string;
	    status: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = source["server"];
	        this.address = source["address"];
	        this.offsetMs = source["offsetMs"];
	        this.delayMs = source["delayMs"];
	        this.stratum = source["stratum"];
	        this.serverTime = source["serverTime"];
	        this.localTime = source["localTime"];
	        this.status = source["status"];
	        this.message = source["message"];
	    }
	}
	export class Report {
	    servers: Server[];
	    level: string;
	    headline: string;
	    advice: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.servers = this.convertValues(source["servers"], Server);
	        this.level = source["level"];
	        this.headline = source["headline"];
	        this.advice = source["advice"];
	        this.checkedAt = source["checkedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace ouidb {
	
	export class Info {
	    mac: string;
	    oui: string;
	    vendor: string;
	    found: boolean;
	    locallyAdministered: boolean;
	    randomized: boolean;
	    multicast: boolean;
	    broadcast: boolean;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mac = source["mac"];
	        this.oui = source["oui"];
	        this.vendor = source["vendor"];
	        this.found = source["found"];
	        this.locallyAdministered = source["locallyAdministered"];
	        this.randomized = source["randomized"];
	        this.multicast = source["multicast"];
	        this.broadcast = source["broadcast"];
	        this.note = source["note"];
	    }
	}
	export class Meta {
	    source: string;
	    fetched: string;
	    records: number;
	
	    static createFrom(source: any = {}) {
	        return new Meta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.fetched = source["fetched"];
	        this.records = source["records"];
	    }
	}

}

export namespace pingmon {
	
	export class Params {
	    targets: string[];
	    intervalMs: number;
	    timeoutMs: number;
	    rounds: number;
	    tcpPort: number;
	    skipTcp: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targets = source["targets"];
	        this.intervalMs = source["intervalMs"];
	        this.timeoutMs = source["timeoutMs"];
	        this.rounds = source["rounds"];
	        this.tcpPort = source["tcpPort"];
	        this.skipTcp = source["skipTcp"];
	    }
	}

}

export namespace portlisten {
	
	export class Params {
	    port: number;
	    protocol: string;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.protocol = source["protocol"];
	    }
	}

}

export namespace portscan {
	
	export class Params {
	    host: string;
	    ports: string;
	    timeoutMs: number;
	    workers: number;
	    grabBanners: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.ports = source["ports"];
	        this.timeoutMs = source["timeoutMs"];
	        this.workers = source["workers"];
	        this.grabBanners = source["grabBanners"];
	    }
	}

}

export namespace pubip {
	
	export class Info {
	    ipv4: string;
	    ipv6: string;
	    reverseDns: string;
	    isp: string;
	    asn: string;
	    city: string;
	    region: string;
	    country: string;
	    countryName: string;
	    timezone: string;
	    latitude: number;
	    longitude: number;
	    source: string;
	    partial: boolean;
	    note: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ipv4 = source["ipv4"];
	        this.ipv6 = source["ipv6"];
	        this.reverseDns = source["reverseDns"];
	        this.isp = source["isp"];
	        this.asn = source["asn"];
	        this.city = source["city"];
	        this.region = source["region"];
	        this.country = source["country"];
	        this.countryName = source["countryName"];
	        this.timezone = source["timezone"];
	        this.latitude = source["latitude"];
	        this.longitude = source["longitude"];
	        this.source = source["source"];
	        this.partial = source["partial"];
	        this.note = source["note"];
	        this.checkedAt = source["checkedAt"];
	    }
	}

}

export namespace qrgen {
	
	export class Code {
	    size: number;
	    version: number;
	    ecLevel: string;
	    mask: number;
	    modules: boolean[];
	    quiet: number;
	    payload: string;
	    payloadBytes: number;
	    capacity: number;
	
	    static createFrom(source: any = {}) {
	        return new Code(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.size = source["size"];
	        this.version = source["version"];
	        this.ecLevel = source["ecLevel"];
	        this.mask = source["mask"];
	        this.modules = source["modules"];
	        this.quiet = source["quiet"];
	        this.payload = source["payload"];
	        this.payloadBytes = source["payloadBytes"];
	        this.capacity = source["capacity"];
	    }
	}
	export class Params {
	    mode: string;
	    text: string;
	    ssid: string;
	    password: string;
	    security: string;
	    hidden: boolean;
	    ecLevel: string;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.text = source["text"];
	        this.ssid = source["ssid"];
	        this.password = source["password"];
	        this.security = source["security"];
	        this.hidden = source["hidden"];
	        this.ecLevel = source["ecLevel"];
	    }
	}

}

export namespace rawprint {
	
	export class Params {
	    host: string;
	    port: number;
	    timeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.timeoutMs = source["timeoutMs"];
	    }
	}
	export class Result {
	    host: string;
	    port: number;
	    address: string;
	    connected: boolean;
	    connectMs: number;
	    printed: boolean;
	    bytesSent: number;
	    reply: string;
	    model: string;
	    statusCode: string;
	    display: string;
	    online: string;
	    level: string;
	    headline: string;
	    advice: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.address = source["address"];
	        this.connected = source["connected"];
	        this.connectMs = source["connectMs"];
	        this.printed = source["printed"];
	        this.bytesSent = source["bytesSent"];
	        this.reply = source["reply"];
	        this.model = source["model"];
	        this.statusCode = source["statusCode"];
	        this.display = source["display"];
	        this.online = source["online"];
	        this.level = source["level"];
	        this.headline = source["headline"];
	        this.advice = source["advice"];
	        this.checkedAt = source["checkedAt"];
	    }
	}

}

export namespace renamer {
	
	export class ApplyItem {
	    old: string;
	    new: string;
	    state: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ApplyItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.old = source["old"];
	        this.new = source["new"];
	        this.state = source["state"];
	        this.reason = source["reason"];
	    }
	}
	export class Rename {
	    from: string;
	    to: string;
	
	    static createFrom(source: any = {}) {
	        return new Rename(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	export class Batch {
	    folder: string;
	    appliedAt: string;
	    renames: Rename[];
	
	    static createFrom(source: any = {}) {
	        return new Batch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.appliedAt = source["appliedAt"];
	        this.renames = this.convertValues(source["renames"], Rename);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ApplyResult {
	    folder: string;
	    renamed: number;
	    failed: number;
	    skipped: number;
	    items: ApplyItem[];
	    batch: Batch;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new ApplyResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.renamed = source["renamed"];
	        this.failed = source["failed"];
	        this.skipped = source["skipped"];
	        this.items = this.convertValues(source["items"], ApplyItem);
	        this.batch = this.convertValues(source["batch"], Batch);
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Params {
	    folder: string;
	    find: string;
	    replace: string;
	    useRegex: boolean;
	    case: string;
	    prefix: string;
	    suffix: string;
	    number: boolean;
	    start: number;
	    step: number;
	    padding: number;
	    keepExtension: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.find = source["find"];
	        this.replace = source["replace"];
	        this.useRegex = source["useRegex"];
	        this.case = source["case"];
	        this.prefix = source["prefix"];
	        this.suffix = source["suffix"];
	        this.number = source["number"];
	        this.start = source["start"];
	        this.step = source["step"];
	        this.padding = source["padding"];
	        this.keepExtension = source["keepExtension"];
	    }
	}
	export class Row {
	    old: string;
	    new: string;
	    kind: string;
	    action: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new Row(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.old = source["old"];
	        this.new = source["new"];
	        this.kind = source["kind"];
	        this.action = source["action"];
	        this.reason = source["reason"];
	    }
	}
	export class Plan {
	    folder: string;
	    fingerprint: string;
	    rows: Row[];
	    changed: number;
	    unchanged: number;
	    skipped: number;
	    blocked: number;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.fingerprint = source["fingerprint"];
	        this.rows = this.convertValues(source["rows"], Row);
	        this.changed = source["changed"];
	        this.unchanged = source["unchanged"];
	        this.skipped = source["skipped"];
	        this.blocked = source["blocked"];
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace selfupdate {
	
	export class Status {
	    current: string;
	    latest: string;
	    url: string;
	    newer: boolean;
	    published: string;
	    note: string;
	    canInstall: boolean;
	    installNote: string;
	    assetName: string;
	    assetSize: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.url = source["url"];
	        this.newer = source["newer"];
	        this.published = source["published"];
	        this.note = source["note"];
	        this.canInstall = source["canInstall"];
	        this.installNote = source["installNote"];
	        this.assetName = source["assetName"];
	        this.assetSize = source["assetSize"];
	    }
	}

}

export namespace sitecheck {
	
	export class Hop {
	    url: string;
	    status: number;
	    location: string;
	
	    static createFrom(source: any = {}) {
	        return new Hop(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.status = source["status"];
	        this.location = source["location"];
	    }
	}
	export class Params {
	    url: string;
	    timeoutMs: number;
	    followRedirects: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.timeoutMs = source["timeoutMs"];
	        this.followRedirects = source["followRedirects"];
	    }
	}
	export class TLSInfo {
	    version: string;
	    cipherSuite: string;
	    subject: string;
	    commonName: string;
	    issuer: string;
	    sans: string[];
	    notBefore: string;
	    notAfter: string;
	    daysRemaining: number;
	    expired: boolean;
	    notYetValid: boolean;
	    hostnameMatch: boolean;
	    chainValid: boolean;
	    chainError: string;
	    selfSigned: boolean;
	    chainSubjects: string[];
	    serialNumber: string;
	    signatureAlgorithm: string;
	    keyType: string;
	    sha256Fingerprint: string;
	
	    static createFrom(source: any = {}) {
	        return new TLSInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.cipherSuite = source["cipherSuite"];
	        this.subject = source["subject"];
	        this.commonName = source["commonName"];
	        this.issuer = source["issuer"];
	        this.sans = source["sans"];
	        this.notBefore = source["notBefore"];
	        this.notAfter = source["notAfter"];
	        this.daysRemaining = source["daysRemaining"];
	        this.expired = source["expired"];
	        this.notYetValid = source["notYetValid"];
	        this.hostnameMatch = source["hostnameMatch"];
	        this.chainValid = source["chainValid"];
	        this.chainError = source["chainError"];
	        this.selfSigned = source["selfSigned"];
	        this.chainSubjects = source["chainSubjects"];
	        this.serialNumber = source["serialNumber"];
	        this.signatureAlgorithm = source["signatureAlgorithm"];
	        this.keyType = source["keyType"];
	        this.sha256Fingerprint = source["sha256Fingerprint"];
	    }
	}
	export class Timing {
	    dnsMs: number;
	    connectMs: number;
	    tlsMs: number;
	    ttfbMs: number;
	    totalMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Timing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dnsMs = source["dnsMs"];
	        this.connectMs = source["connectMs"];
	        this.tlsMs = source["tlsMs"];
	        this.ttfbMs = source["ttfbMs"];
	        this.totalMs = source["totalMs"];
	    }
	}
	export class Result {
	    url: string;
	    finalUrl: string;
	    status: number;
	    statusText: string;
	    level: string;
	    headline: string;
	    ip: string;
	    serverHeader: string;
	    bodyBytes: number;
	    redirects: Hop[];
	    timing: Timing;
	    tls?: TLSInfo;
	    warnings: string[];
	    errors: string[];
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.finalUrl = source["finalUrl"];
	        this.status = source["status"];
	        this.statusText = source["statusText"];
	        this.level = source["level"];
	        this.headline = source["headline"];
	        this.ip = source["ip"];
	        this.serverHeader = source["serverHeader"];
	        this.bodyBytes = source["bodyBytes"];
	        this.redirects = this.convertValues(source["redirects"], Hop);
	        this.timing = this.convertValues(source["timing"], Timing);
	        this.tls = this.convertValues(source["tls"], TLSInfo);
	        this.warnings = source["warnings"];
	        this.errors = source["errors"];
	        this.checkedAt = source["checkedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace speedtest {
	
	export class Params {
	    durationSec: number;
	    streams: number;
	    skipUpload: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.durationSec = source["durationSec"];
	        this.streams = source["streams"];
	        this.skipUpload = source["skipUpload"];
	    }
	}

}

export namespace startup {
	
	export class Item {
	    name: string;
	    kind: string;
	    source: string;
	    command: string;
	    publisher: string;
	    startMode: string;
	    state: string;
	    enabled: boolean;
	    concern: string;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.source = source["source"];
	        this.command = source["command"];
	        this.publisher = source["publisher"];
	        this.startMode = source["startMode"];
	        this.state = source["state"];
	        this.enabled = source["enabled"];
	        this.concern = source["concern"];
	    }
	}
	export class Report {
	    os: string;
	    items: Item[];
	    unsupported: string[];
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.items = this.convertValues(source["items"], Item);
	        this.unsupported = source["unsupported"];
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace swlist {
	
	export class Program {
	    name: string;
	    version: string;
	    publisher: string;
	    installedOn: string;
	    sizeBytes: number;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new Program(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.publisher = source["publisher"];
	        this.installedOn = source["installedOn"];
	        this.sizeBytes = source["sizeBytes"];
	        this.source = source["source"];
	    }
	}
	export class Report {
	    os: string;
	    programs: Program[];
	    sources: string[];
	    unsupported: string[];
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.programs = this.convertValues(source["programs"], Program);
	        this.sources = source["sources"];
	        this.unsupported = source["unsupported"];
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace sysinfo {
	
	export class Disk {
	    mount: string;
	    fs: string;
	    total: number;
	    used: number;
	    free: number;
	    usedPct: number;
	
	    static createFrom(source: any = {}) {
	        return new Disk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mount = source["mount"];
	        this.fs = source["fs"];
	        this.total = source["total"];
	        this.used = source["used"];
	        this.free = source["free"];
	        this.usedPct = source["usedPct"];
	    }
	}
	export class Report {
	    hostname: string;
	    user: string;
	    os: string;
	    osName: string;
	    osVersion: string;
	    arch: string;
	    manufacturer: string;
	    model: string;
	    serial: string;
	    cpuModel: string;
	    cpuCores: number;
	    memoryTotal: number;
	    memoryFree: number;
	    uptimeS: number;
	    bootTime: string;
	    appVersion: string;
	    disks: Disk[];
	    unsupported: string[];
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostname = source["hostname"];
	        this.user = source["user"];
	        this.os = source["os"];
	        this.osName = source["osName"];
	        this.osVersion = source["osVersion"];
	        this.arch = source["arch"];
	        this.manufacturer = source["manufacturer"];
	        this.model = source["model"];
	        this.serial = source["serial"];
	        this.cpuModel = source["cpuModel"];
	        this.cpuCores = source["cpuCores"];
	        this.memoryTotal = source["memoryTotal"];
	        this.memoryFree = source["memoryFree"];
	        this.uptimeS = source["uptimeS"];
	        this.bootTime = source["bootTime"];
	        this.appVersion = source["appVersion"];
	        this.disks = this.convertValues(source["disks"], Disk);
	        this.unsupported = source["unsupported"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace tlsprobe {
	
	export class Attempt {
	    version: string;
	    testable: boolean;
	    accepted: boolean;
	    cipher: string;
	    alpn: string;
	    message: string;
	    handshakeMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Attempt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.testable = source["testable"];
	        this.accepted = source["accepted"];
	        this.cipher = source["cipher"];
	        this.alpn = source["alpn"];
	        this.message = source["message"];
	        this.handshakeMs = source["handshakeMs"];
	    }
	}
	export class Params {
	    target: string;
	    timeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.timeoutMs = source["timeoutMs"];
	    }
	}
	export class Report {
	    target: string;
	    host: string;
	    port: number;
	    ip: string;
	    attempts: Attempt[];
	    level: string;
	    headline: string;
	    advice: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.ip = source["ip"];
	        this.attempts = this.convertValues(source["attempts"], Attempt);
	        this.level = source["level"];
	        this.headline = source["headline"];
	        this.advice = source["advice"];
	        this.checkedAt = source["checkedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace totp {
	
	export class Account {
	    id: string;
	    issuer: string;
	    label: string;
	    digits: number;
	    period: number;
	    algorithm: string;
	    addedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.issuer = source["issuer"];
	        this.label = source["label"];
	        this.digits = source["digits"];
	        this.period = source["period"];
	        this.algorithm = source["algorithm"];
	        this.addedAt = source["addedAt"];
	    }
	}
	export class AccountList {
	    accounts: Account[];
	
	    static createFrom(source: any = {}) {
	        return new AccountList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accounts = this.convertValues(source["accounts"], Account);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Code {
	    id: string;
	    issuer: string;
	    label: string;
	    code: string;
	    digits: number;
	    period: number;
	    expiresIn: number;
	
	    static createFrom(source: any = {}) {
	        return new Code(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.issuer = source["issuer"];
	        this.label = source["label"];
	        this.code = source["code"];
	        this.digits = source["digits"];
	        this.period = source["period"];
	        this.expiresIn = source["expiresIn"];
	    }
	}
	export class CodeSet {
	    codes: Code[];
	    atUnix: number;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new CodeSet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.codes = this.convertValues(source["codes"], Code);
	        this.atUnix = source["atUnix"];
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportReport {
	    added: number;
	    skipped: number;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.skipped = source["skipped"];
	        this.note = source["note"];
	    }
	}
	export class NewAccount {
	    uri: string;
	    secret: string;
	    issuer: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new NewAccount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.secret = source["secret"];
	        this.issuer = source["issuer"];
	        this.label = source["label"];
	    }
	}
	export class Status {
	    hasVault: boolean;
	    unlocked: boolean;
	    accounts: number;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasVault = source["hasVault"];
	        this.unlocked = source["unlocked"];
	        this.accounts = source["accounts"];
	        this.note = source["note"];
	    }
	}

}

export namespace tracert {
	
	export class Params {
	    host: string;
	    maxHops: number;
	    queries: number;
	    timeoutMs: number;
	    noNames: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.maxHops = source["maxHops"];
	        this.queries = source["queries"];
	        this.timeoutMs = source["timeoutMs"];
	        this.noNames = source["noNames"];
	    }
	}

}

export namespace triage {
	
	export class Params {
	    timeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeoutMs = source["timeoutMs"];
	    }
	}

}

export namespace urlcheck {
	
	export class Age {
	    known: boolean;
	    registered: string;
	    days: number;
	    human: string;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Age(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.known = source["known"];
	        this.registered = source["registered"];
	        this.days = source["days"];
	        this.human = source["human"];
	        this.note = source["note"];
	    }
	}
	export class Finding {
	    id: string;
	    severity: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new Finding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.severity = source["severity"];
	        this.text = source["text"];
	    }
	}
	export class Hop {
	    n: number;
	    url: string;
	    host: string;
	    method: string;
	    headRejected: boolean;
	    status: number;
	    location: string;
	    next: string;
	    tookMs: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Hop(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.n = source["n"];
	        this.url = source["url"];
	        this.host = source["host"];
	        this.method = source["method"];
	        this.headRejected = source["headRejected"];
	        this.status = source["status"];
	        this.location = source["location"];
	        this.next = source["next"];
	        this.tookMs = source["tookMs"];
	        this.error = source["error"];
	    }
	}
	export class HostName {
	    raw: string;
	    decoded: string;
	    punycode: boolean;
	    registrable: string;
	    isIp: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostName(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.raw = source["raw"];
	        this.decoded = source["decoded"];
	        this.punycode = source["punycode"];
	        this.registrable = source["registrable"];
	        this.isIp = source["isIp"];
	    }
	}
	export class Params {
	    url: string;
	    timeoutMs: number;
	    skipAge: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.timeoutMs = source["timeoutMs"];
	        this.skipAge = source["skipAge"];
	    }
	}
	export class Unwrap {
	    wrapper: string;
	    from: string;
	    to: string;
	
	    static createFrom(source: any = {}) {
	        return new Unwrap(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.wrapper = source["wrapper"];
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	export class Report {
	    input: string;
	    start: string;
	    final: string;
	    unwrapped: Unwrap[];
	    hops: Hop[];
	    startHost: HostName;
	    finalHost: HostName;
	    findings: Finding[];
	    age: Age;
	    level: string;
	    headline: string;
	    stopped: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.input = source["input"];
	        this.start = source["start"];
	        this.final = source["final"];
	        this.unwrapped = this.convertValues(source["unwrapped"], Unwrap);
	        this.hops = this.convertValues(source["hops"], Hop);
	        this.startHost = this.convertValues(source["startHost"], HostName);
	        this.finalHost = this.convertValues(source["finalHost"], HostName);
	        this.findings = this.convertValues(source["findings"], Finding);
	        this.age = this.convertValues(source["age"], Age);
	        this.level = source["level"];
	        this.headline = source["headline"];
	        this.stopped = source["stopped"];
	        this.checkedAt = source["checkedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace usbhist {
	
	export class Device {
	    name: string;
	    manufacturer: string;
	    vendorId: string;
	    productId: string;
	    serial: string;
	    kind: string;
	    connected: boolean;
	    firstSeen: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.manufacturer = source["manufacturer"];
	        this.vendorId = source["vendorId"];
	        this.productId = source["productId"];
	        this.serial = source["serial"];
	        this.kind = source["kind"];
	        this.connected = source["connected"];
	        this.firstSeen = source["firstSeen"];
	        this.source = source["source"];
	    }
	}
	export class Report {
	    os: string;
	    devices: Device[];
	    history: boolean;
	    unsupported: string[];
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.devices = this.convertValues(source["devices"], Device);
	        this.history = source["history"];
	        this.unsupported = source["unsupported"];
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace wifi {
	
	export class Link {
	    interface: string;
	    connected: boolean;
	    ssid: string;
	    bssid: string;
	    band: string;
	    channel: number;
	    frequencyMhz: number;
	    widthMhz: number;
	    signalDbm: number;
	    signalPercent: number;
	    rxMbps: number;
	    txMbps: number;
	    security: string;
	    reading: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new Link(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.interface = source["interface"];
	        this.connected = source["connected"];
	        this.ssid = source["ssid"];
	        this.bssid = source["bssid"];
	        this.band = source["band"];
	        this.channel = source["channel"];
	        this.frequencyMhz = source["frequencyMhz"];
	        this.widthMhz = source["widthMhz"];
	        this.signalDbm = source["signalDbm"];
	        this.signalPercent = source["signalPercent"];
	        this.rxMbps = source["rxMbps"];
	        this.txMbps = source["txMbps"];
	        this.security = source["security"];
	        this.reading = source["reading"];
	        this.source = source["source"];
	    }
	}
	export class Report {
	    os: string;
	    links: Link[];
	    unsupported: string[];
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.links = this.convertValues(source["links"], Link);
	        this.unsupported = source["unsupported"];
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace wol {
	
	export class Awake {
	    ip: string;
	    alive: boolean;
	    via: string;
	    latencyMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Awake(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.alive = source["alive"];
	        this.via = source["via"];
	        this.latencyMs = source["latencyMs"];
	    }
	}
	export class Send {
	    adapter: string;
	    from: string;
	    to: string;
	    port: number;
	    bytes: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Send(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.adapter = source["adapter"];
	        this.from = source["from"];
	        this.to = source["to"];
	        this.port = source["port"];
	        this.bytes = source["bytes"];
	        this.error = source["error"];
	    }
	}
	export class Target {
	    adapter: string;
	    from: string;
	    to: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new Target(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.adapter = source["adapter"];
	        this.from = source["from"];
	        this.to = source["to"];
	        this.port = source["port"];
	    }
	}
	export class TargetList {
	    targets: Target[];
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new TargetList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targets = this.convertValues(source["targets"], Target);
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WakeParams {
	    mac: string;
	    broadcast: string;
	    port: number;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new WakeParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mac = source["mac"];
	        this.broadcast = source["broadcast"];
	        this.port = source["port"];
	        this.password = source["password"];
	    }
	}
	export class WakeResult {
	    mac: string;
	    vendor: string;
	    sent: Send[];
	    failed: Send[];
	    packetHex: string;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new WakeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mac = source["mac"];
	        this.vendor = source["vendor"];
	        this.sent = this.convertValues(source["sent"], Send);
	        this.failed = this.convertValues(source["failed"], Send);
	        this.packetHex = source["packetHex"];
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}


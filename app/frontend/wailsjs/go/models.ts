export namespace profile {
	
	export class Entry {
	    id: string;
	    name?: string;
	    extends?: string;
	    enabled?: boolean;
	    order?: number;
	    cmd?: string;
	    args?: string[];
	    env?: Record<string, string>;
	    status?: string;
	    setupHooks?: string;
	    idleSeconds?: number;
	    attentionPatterns?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.extends = source["extends"];
	        this.enabled = source["enabled"];
	        this.order = source["order"];
	        this.cmd = source["cmd"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.status = source["status"];
	        this.setupHooks = source["setupHooks"];
	        this.idleSeconds = source["idleSeconds"];
	        this.attentionPatterns = source["attentionPatterns"];
	    }
	}
	export class Profile {
	    id: string;
	    name: string;
	    cmd: string;
	    args?: string[];
	    env?: Record<string, string>;
	    status: string;
	    setupHooks?: string;
	    attentionPatterns?: string[];
	    idleSeconds?: number;
	    source: string;
	    extends?: string;
	    enabled: boolean;
	    installed: boolean;
	    path?: string;
	    homepage?: string;
	    install?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.cmd = source["cmd"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.status = source["status"];
	        this.setupHooks = source["setupHooks"];
	        this.attentionPatterns = source["attentionPatterns"];
	        this.idleSeconds = source["idleSeconds"];
	        this.source = source["source"];
	        this.extends = source["extends"];
	        this.enabled = source["enabled"];
	        this.installed = source["installed"];
	        this.path = source["path"];
	        this.homepage = source["homepage"];
	        this.install = source["install"];
	        this.error = source["error"];
	    }
	}

}

export namespace project {
	
	export class Project {
	    id: string;
	    name: string;
	    path: string;
	    // Go type: time
	    addedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.addedAt = this.convertValues(source["addedAt"], null);
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

export namespace protocol {
	
	export class CatalogInfo {
	    id: string;
	    name: string;
	    binaries?: string[];
	    cmd?: string;
	    args?: string[];
	    status: string;
	    setupHooks?: string;
	    attentionPatterns?: string[];
	    homepage?: string;
	    install?: string;
	    command: string;
	    installed: boolean;
	    path?: string;
	
	    static createFrom(source: any = {}) {
	        return new CatalogInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.binaries = source["binaries"];
	        this.cmd = source["cmd"];
	        this.args = source["args"];
	        this.status = source["status"];
	        this.setupHooks = source["setupHooks"];
	        this.attentionPatterns = source["attentionPatterns"];
	        this.homepage = source["homepage"];
	        this.install = source["install"];
	        this.command = source["command"];
	        this.installed = source["installed"];
	        this.path = source["path"];
	    }
	}
	export class Progress {
	    state: string;
	    value: number;
	
	    static createFrom(source: any = {}) {
	        return new Progress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.value = source["value"];
	    }
	}
	export class SessionInfo {
	    id: string;
	    projectId: string;
	    projectDir: string;
	    profileId: string;
	    projectName?: string;
	    cmd: string;
	    args?: string[];
	    cwd: string;
	    pid: number;
	    // Go type: time
	    createdAt: any;
	    state: string;
	    exitCode?: number;
	    cols: number;
	    rows: number;
	    attentionKind?: string;
	    attentionMessage?: string;
	    // Go type: time
	    attentionAt?: any;
	    unread?: boolean;
	    activity?: string;
	    // Go type: time
	    activityAt?: any;
	    activitySource?: string;
	    // Go type: time
	    turnStartedAt?: any;
	    // Go type: time
	    turnEndedAt?: any;
	    // Go type: time
	    lastOutputAt?: any;
	    // Go type: time
	    lastEventAt?: any;
	    stalled?: boolean;
	    // Go type: time
	    stalledAt?: any;
	    // Go type: Progress
	    progress?: any;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.projectDir = source["projectDir"];
	        this.profileId = source["profileId"];
	        this.projectName = source["projectName"];
	        this.cmd = source["cmd"];
	        this.args = source["args"];
	        this.cwd = source["cwd"];
	        this.pid = source["pid"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.state = source["state"];
	        this.exitCode = source["exitCode"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	        this.attentionKind = source["attentionKind"];
	        this.attentionMessage = source["attentionMessage"];
	        this.attentionAt = this.convertValues(source["attentionAt"], null);
	        this.unread = source["unread"];
	        this.activity = source["activity"];
	        this.activityAt = this.convertValues(source["activityAt"], null);
	        this.activitySource = source["activitySource"];
	        this.turnStartedAt = this.convertValues(source["turnStartedAt"], null);
	        this.turnEndedAt = this.convertValues(source["turnEndedAt"], null);
	        this.lastOutputAt = this.convertValues(source["lastOutputAt"], null);
	        this.lastEventAt = this.convertValues(source["lastEventAt"], null);
	        this.stalled = source["stalled"];
	        this.stalledAt = this.convertValues(source["stalledAt"], null);
	        this.progress = this.convertValues(source["progress"], null);
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

export namespace settings {
	
	export class Activity {
	    stallSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new Activity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stallSeconds = source["stallSeconds"];
	    }
	}
	export class Notify {
	    enabled: boolean;
	    sound: boolean;
	    notifyOnDone: boolean;
	    notifyOnStall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Notify(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.sound = source["sound"];
	        this.notifyOnDone = source["notifyOnDone"];
	        this.notifyOnStall = source["notifyOnStall"];
	    }
	}
	export class Settings {
	    notify: Notify;
	    activity: Activity;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.notify = this.convertValues(source["notify"], Notify);
	        this.activity = this.convertValues(source["activity"], Activity);
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


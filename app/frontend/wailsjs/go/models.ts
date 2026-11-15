export namespace profile {
	
	export class Profile {
	    id: string;
	    cmd: string;
	    args?: string[];
	    env?: Record<string, string>;
	    status: string;
	    setupHooks?: string;
	    idleSeconds?: number;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.cmd = source["cmd"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.status = source["status"];
	        this.setupHooks = source["setupHooks"];
	        this.idleSeconds = source["idleSeconds"];
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
	
	export class SessionInfo {
	    id: string;
	    projectId: string;
	    projectDir: string;
	    profileId: string;
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
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.projectDir = source["projectDir"];
	        this.profileId = source["profileId"];
	        this.cmd = source["cmd"];
	        this.args = source["args"];
	        this.cwd = source["cwd"];
	        this.pid = source["pid"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.state = source["state"];
	        this.exitCode = source["exitCode"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
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


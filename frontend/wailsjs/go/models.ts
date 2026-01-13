export namespace protocol {
	
	export class Message {
	    id: number;
	    tp: string;
	    u: string;
	    t: string;
	    h: string;
	    n: string;
	    tm: string;
	    ah: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.tp = source["tp"];
	        this.u = source["u"];
	        this.t = source["t"];
	        this.h = source["h"];
	        this.n = source["n"];
	        this.tm = source["tm"];
	        this.ah = source["ah"];
	    }
	}

}


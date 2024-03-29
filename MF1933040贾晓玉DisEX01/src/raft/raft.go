package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import "sync"
import "labrpc"
import "math/rand"
import "time"
import "sync/atomic"
// import "bytes"
// import "encoding/gob"

const (
	FOLLOWER = iota
	CANDIDATE
	LEADER
	
	HEARBEAT_INTERVAL = 50
	MIN_ELECTION_INTERVAL = 150
	MAX_ELECTION_INTERVAL = 300
)

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex
	peers     []*labrpc.ClientEnd
	persister *Persister
	me        int // index into peers[]

	// Your data here.
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	votedFor      int  //index of candidate to vote for
	voteAcquired  int  //the number of votes from others
	state         int32
	currentTerm   int32
	
	electionTimer *time.Timer
	voteCh        chan struct{}  //the signal of successful voting
	appendCh      chan struct{}  //the signal of updating the log successfully

}

//atomic operations
func (rf *Raft) getTerm() int32 {
	return atomic.LoadInt32(&rf.currentTerm)
}

func (rf *Raft) incrementTerm() {
	atomic.AddInt32(&rf.currentTerm, 1)
}

func (rf *Raft) isState(state int32) bool {
	return atomic.LoadInt32(&rf.state) == state
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here.
	term = int(rf.getTerm())
	isleader = rf.isState(LEADER)
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here.
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) { //??????????
	// Your code here.
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
	if(data == nil || len(data) < 1) { //not containing any state
		return
	}
}


func (rf *Raft) updateStateTo(state int32) { //transfer the state 
	if rf.isState(state) { //same to the state to transfer
		return
	}
	switch state {
		case FOLLOWER:
			rf.state = FOLLOWER
			rf.votedFor = -1
		case CANDIDATE:
			rf.state = CANDIDATE
			rf.startElection()
		case LEADER:
			rf.state = LEADER
	}
}

//
// example RequestVote RPC arguments structure.
//
type RequestVoteArgs struct {
	// Your data here.
	Term         int32   //current term of requesting candidate
	CandidateId  int   //the id of the requester
}

//
// example RequestVote RPC reply structure.
//
type RequestVoteReply struct {
	// Your data here.
	Term         int32  //record the max(requesters'term, its term)
	VoteGranted  bool //vote or not
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) { //RequestVote handdler
	// Your code here.
	rf.mu.Lock()   //get the lock
	defer rf.mu.Unlock()
	if args.Term < rf.currentTerm {
		reply.VoteGranted = false
		reply.Term = rf.currentTerm
	}else if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.updateStateTo(FOLLOWER)
		rf.votedFor = args.CandidateId
		reply.VoteGranted = true
	}else {
		if rf.votedFor == -1 {//haven't vote for anyone
			rf.votedFor = args.CandidateId
			reply.VoteGranted = true
		}else {
			reply.VoteGranted = false
		}
	}
	if reply.VoteGranted == true { // vote for current requester
		go func() { rf.voteCh <- struct{}{} }()  //send the struct{}{} to the voteCh channel
	}	
}

type AppendEntriesArgs struct { //the information sending by leader
	Term      int32
	LeaderId  int
}

type AppendEntriesReply struct { //the response for the AppendEntriesArgs
	Term      int32
	Success   bool
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) { // AppendingEntries handler
	rf.mu.Lock()
	defer rf.mu.Unlock()
	
	if args.Term <rf.currentTerm {
		reply.Success = false
		reply.Term = rf.currentTerm
	}else if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.updateStateTo(FOLLOWER)
		reply.Success = true
	}else {
		reply.Success = true
	}
	go func() { rf.appendCh <- struct{}{} }() //send the struct{} to the appendCh channel
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// returns true if labrpc says the RPC was delivered.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}
func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
//send heartbeat
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}
//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true


	return index, term, isLeader
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here.
	rf.state = FOLLOWER
	rf.votedFor = -1
	rf.voteCh = make(chan struct{}) //define the v of the channal is the size of an empty struct{}
	rf.appendCh = make(chan struct{})
	
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	
	go rf.startLoop() // start election


	return rf
}

func randElectionDuration() time.Duration { //produce the randtime between MIN_ELECTION_INTERVAL and MAX_ELECTION_INTERVAL to control the election time
	r:= rand.New(rand.NewSource(time.Now().UnixNano()))
	return time.Millisecond * time.Duration(r.Int63n(MAX_ELECTION_INTERVAL-MIN_ELECTION_INTERVAL)+MIN_ELECTION_INTERVAL)
}

func (rf *Raft) broadcastVoteReq() { //the candidator broadcast the votes to others
	if !rf.isState(CANDIDATE) {
		return
	}
	args := RequestVoteArgs{Term: atomic.LoadInt32(&rf.currentTerm), CandidateId:rf.me} //the infromation from the candidator who send the requested vote
	for i, _ := range rf.peers { 
		if i == rf.me { //ignore itself
			continue
		}
		go func(server int) {
			var reply RequestVoteReply
			if rf.sendRequestVote(server, &args, &reply) {//current request server is a candidator and could send the voteReq
				rf.mu.Lock()
				defer rf.mu.Unlock()
				
				if reply.VoteGranted == true { //reply successfully
					rf.voteAcquired += 1
				}else {
					if reply.Term > rf.currentTerm {//the request's term lastest than the requester's
						rf.currentTerm = reply.Term
						rf.updateStateTo(FOLLOWER)
					}
				}
			}
		}(i)//send the voteReq to the ith server
	}
}

func (rf *Raft) broadcastAppendEntries() { //send the heartbeats
	if rf.isState(LEADER){
		return
	}
	args := AppendEntriesArgs{Term: atomic.LoadInt32(&rf.currentTerm), LeaderId: rf.me} //the information of the leader
	for i, _ := range rf.peers {
		if i == rf.me {
			continue
		}
		go func(server int) {
			var reply AppendEntriesReply
			if rf.sendAppendEntries(server, &args, &reply) {//leader and could send the heartbeats
				rf.mu.Lock()
				defer rf.mu.Unlock()
				if reply.Success == true {
					
				}else {
					if reply.Term > rf.currentTerm {
						rf.currentTerm = reply.Term
						rf.updateStateTo(FOLLOWER)
					}
				}
			}
		}(i)
	}
}

func (rf* Raft) startElection() {
	rf.incrementTerm() //update the term 
	rf.electionTimer.Reset(randElectionDuration())//reset thr election interval
	rf.votedFor = rf.me //vote for itself
	rf.voteAcquired = 1 //the number of vote add to 1
	rf.broadcastVoteReq() //send the request to others
}

func (rf *Raft) startLoop() {
	rf.electionTimer = time.NewTimer(randElectionDuration())
	for {
		switch rf.state {
			case FOLLOWER:
				select {
					case <-rf.voteCh: //election state:recieve the data from the voteCh channel
						rf.electionTimer.Reset(randElectionDuration())
					case <-rf.appendCh: //recieve the state of appending to the channel
						rf.electionTimer.Reset(randElectionDuration())
					case <-rf.electionTimer.C: //running time is longer than the electionTime, start an new election
						rf.mu.Lock()
						rf.updateStateTo(CANDIDATE)
						rf.mu.Unlock()
				}
			case CANDIDATE:
				rf.mu.Lock()
				select {
					case <-rf.appendCh:  // recieve the leaders' information
						rf.updateStateTo(FOLLOWER)
					case <-rf.electionTimer.C: // over the election time
						rf.startElection()
					default:
						// check if it has got enough votes
						if rf.voteAcquired > len(rf.peers)/2 {
							rf.updateStateTo(LEADER)
						}
				}
				rf.mu.Unlock()
			case LEADER:
				rf.broadcastAppendEntries()
				time.Sleep(HEARBEAT_INTERVAL * time.Millisecond)
		}
	}
}
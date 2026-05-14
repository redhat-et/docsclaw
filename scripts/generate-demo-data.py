#!/usr/bin/env python3
"""
Generate synthetic test data for DocsClaw batch processing demos.

Produces three JSON files with realistic test documents:
- hr-documents.json: 1 job description + 100 resumes with controlled match distribution
- security-documents.json: vulnerability scan + asset inventory with SLA violations
- finance-documents.json: contracts + invoices with planted anomalies
"""

import json
import os
import random
from datetime import datetime, timedelta
from pathlib import Path


def main():
    """Generate all demo data files."""
    random.seed(42)

    output_dir = Path("demo/batch/data")
    output_dir.mkdir(parents=True, exist_ok=True)

    # Generate each data set
    hr_docs = generate_hr_documents()
    security_docs = generate_security_documents()
    finance_docs = generate_finance_documents()

    # Write to files
    write_json(output_dir / "hr-documents.json", hr_docs)
    write_json(output_dir / "security-documents.json", security_docs)
    write_json(output_dir / "finance-documents.json", finance_docs)

    # Print summary
    print(f"Generated demo data in {output_dir}/")
    print(f"  HR documents: {len(hr_docs)} (1 job description + 100 resumes)")
    print(f"    - Strong matches: 20")
    print(f"    - Moderate matches: 30")
    print(f"    - Weak matches: 30")
    print(f"    - Poor matches: 20")
    print(f"  Security documents: {len(security_docs)} (vulnerability scan + asset inventory)")
    print(f"  Finance documents: {len(finance_docs)} (5 contracts + 15 invoices)")


def write_json(path, data):
    """Write data to JSON file with pretty formatting."""
    with open(path, 'w') as f:
        json.dump(data, f, indent=2)


def generate_hr_documents():
    """Generate job description and 100 resumes with controlled distribution."""
    docs = []

    # Job description
    docs.append({
        "id": "DOC-JD001",
        "title": "Job Description - Senior Product Manager - API Platform",
        "sensitivity": "internal",
        "required_department": "hr",
        "content": """POSITION: Senior Product Manager - API Platform
LOCATION: Remote (US/Europe)
DEPARTMENT: Product Management

ABOUT THE ROLE:
We are seeking an experienced Senior Product Manager to lead our API Platform initiative. You will own the product vision, roadmap, and execution for our developer-facing API products serving millions of requests per day.

REQUIREMENTS:
- 7+ years of product management experience
- Deep experience with API products, developer platforms, or platform-as-a-service
- Proven track record of shipping API products at scale
- Strong technical background with ability to engage with engineering teams
- Experience leading cross-functional teams
- Excellent written and verbal communication skills
- Data-driven decision making approach

PREFERRED QUALIFICATIONS:
- Experience with REST, GraphQL, or gRPC APIs
- Background in developer tools or infrastructure products
- Experience with cloud platforms (AWS, GCP, Azure)
- Technical degree in Computer Science or related field

RESPONSIBILITIES:
- Define product vision and strategy for API platform
- Build and maintain product roadmap
- Work closely with engineering, design, and go-to-market teams
- Analyze user feedback and usage data to drive decisions
- Lead cross-functional planning and execution
- Represent product externally to customers and partners"""
    })

    # Generate 100 resumes with controlled distribution
    candidates = []

    # 20 strong matches
    for i in range(20):
        candidates.append(generate_resume(i + 1, "strong"))

    # 30 moderate matches
    for i in range(30):
        candidates.append(generate_resume(i + 21, "moderate"))

    # 30 weak matches
    for i in range(30):
        candidates.append(generate_resume(i + 51, "weak"))

    # 20 poor matches
    for i in range(20):
        candidates.append(generate_resume(i + 81, "poor"))

    docs.extend(candidates)
    return docs


def generate_resume(num, match_level):
    """Generate a single resume with specified match level."""
    name = get_candidate_name(num)

    if match_level == "strong":
        work_history = generate_strong_match_history()
        skills = ["Product Management", "API Design", "REST APIs", "GraphQL", "Platform Strategy",
                 "Cross-functional Leadership", "Agile", "Developer Tools", "Cloud Platforms", "Data Analysis"]
        education = "BS Computer Science, " + random.choice(["MIT", "Stanford", "Carnegie Mellon", "UC Berkeley"])
        summary = f"Senior Product Manager with {random.randint(8, 12)} years of experience leading API platforms and developer products. Proven track record of shipping products at scale with focus on developer experience."

    elif match_level == "moderate":
        work_history = generate_moderate_match_history()
        skills = ["Product Management", "SaaS Products", "Agile", "Stakeholder Management",
                 "Roadmap Planning", "User Research", "SQL", "Analytics"]
        education = "BA Business Administration, " + random.choice(["Boston University", "University of Texas", "Penn State"])
        summary = f"Product Manager with {random.randint(4, 7)} years of experience in SaaS products. Strong analytical skills and cross-functional collaboration."

    elif match_level == "weak":
        work_history = generate_weak_match_history()
        skills = ["Project Management", "Agile", "JIRA", "Confluence", "Stakeholder Communication",
                 "Requirements Gathering", "Risk Management", "Budget Management"]
        education = "BS Business, " + random.choice(["Arizona State", "Ohio State", "University of Florida"])
        summary = f"Project Manager with {random.randint(5, 9)} years coordinating technical projects and leading teams."

    else:  # poor match
        work_history = generate_poor_match_history()
        skills_options = [
            ["JavaScript", "React", "Node.js", "PostgreSQL", "Git", "Docker"],
            ["Figma", "Sketch", "User Research", "Wireframing", "Prototyping", "Design Systems"],
            ["Python", "Machine Learning", "SQL", "Tableau", "Statistics", "A/B Testing"]
        ]
        skills = random.choice(skills_options)
        education = "BS " + random.choice(["Computer Science", "Design", "Mathematics"]) + ", " + random.choice(["State University", "Tech Institute"])
        summary = random.choice([
            f"Software Engineer with {random.randint(3, 6)} years building web applications.",
            f"Product Designer with {random.randint(4, 7)} years creating user experiences.",
            f"Data Scientist with {random.randint(3, 5)} years analyzing product metrics."
        ])

    return {
        "id": f"DOC-R{num:03d}",
        "title": f"Resume - {name}",
        "sensitivity": "confidential",
        "required_department": "hr",
        "content": format_resume(name, summary, work_history, education, skills)
    }


def get_candidate_name(num):
    """Get diverse international name for candidate."""
    names = [
        "Aisha Okafor", "Wei Chen", "Carlos Mendoza", "Priya Sharma", "John Anderson",
        "Fatima Al-Rashid", "Yuki Tanaka", "Maria Silva", "Ahmed Hassan", "Emma Johnson",
        "Dmitri Volkov", "Lakshmi Reddy", "Jean-Pierre Dubois", "Zahra Hosseini", "Michael O'Brien",
        "Sandeep Kumar", "Isabella Romano", "Kwame Mensah", "Mei Lin", "Omar Khalil",
        "Sofia Kowalski", "Raj Patel", "Anna Petrov", "Luis Rodriguez", "Fatou Diallo",
        "Henrik Nielsen", "Amara Okonkwo", "Jin Park", "Leila Mansour", "Thomas Murphy",
        "Ananya Gupta", "Marco Rossi", "Chioma Eze", "Hiroshi Sato", "Beatriz Santos",
        "Nabil Idris", "Olga Ivanova", "Ravi Krishnan", "Camila Fernandez", "Ali Mahmoud",
        "Nina Kowalczyk", "Arjun Mehta", "Elena Popescu", "Diego Martinez", "Ayesha Khan",
        "Viktor Sokolov", "Nisha Desai", "Antoine Leroy", "Yasmin Abdullah", "Sean Kelly",
        "Deepa Iyer", "Giovanni Bianchi", "Nneka Obi", "Kenji Yamamoto", "Ana Costa",
        "Karim Benali", "Svetlana Sokolova", "Vikram Nair", "Laura Gomez", "Hassan Ali",
        "Katarzyna Nowak", "Sanjay Reddy", "Natalia Kozlov", "Roberto Alvarez", "Zainab Farah",
        "Andrei Petrov", "Kavita Singh", "Pierre Martin", "Mariam Youssef", "Patrick Walsh",
        "Anjali Verma", "Luca Ferrari", "Adaeze Okafor", "Takeshi Suzuki", "Gabriela Lima",
        "Tariq Rashid", "Irina Volkova", "Ashok Kumar", "Daniela Ruiz", "Mustafa Kaya",
        "Magdalena Wiśniewska", "Rahul Kapoor", "Ekaterina Ivanov", "Javier Morales", "Samira Noor",
        "Maxim Volkov", "Divya Pillai", "François Lefebvre", "Hana Mansour", "Conor Ryan",
        "Shreya Joshi", "Matteo Conti", "Chiamaka Eze", "Haruto Nakamura", "Juliana Pereira",
        "Rashid Hamdan", "Yulia Petrova", "Suresh Menon", "Valentina Torres", "Ibrahim Diab"
    ]
    return names[(num - 1) % len(names)]


def generate_strong_match_history():
    """Generate work history for strong PM match."""
    companies = ["TechFlow", "CloudScale Systems", "DevHub", "ApiWorks", "Platform.io"]

    return [
        {
            "title": "Senior Product Manager - API Platform",
            "company": companies[0],
            "duration": "2019-Present",
            "description": "Lead API platform serving 500K+ developers. Shipped GraphQL gateway, rate limiting v2, and developer portal. Grew API adoption 300% YoY."
        },
        {
            "title": "Product Manager - Developer Tools",
            "company": companies[1],
            "duration": "2016-2019",
            "description": "Owned SDK and CLI product line. Launched REST API v3 with 99.99% SLA. Led team of 12 engineers and 2 designers."
        },
        {
            "title": "Associate Product Manager",
            "company": companies[2],
            "duration": "2014-2016",
            "description": "Shipped developer documentation platform and API analytics dashboard. Conducted user research with 100+ developers."
        }
    ]


def generate_moderate_match_history():
    """Generate work history for moderate PM match."""
    companies = ["SaaSCorp", "DataFlow", "WebApps Inc", "CloudFirst"]

    return [
        {
            "title": "Product Manager",
            "company": companies[0],
            "duration": "2020-Present",
            "description": "Own B2B SaaS platform roadmap. Launched 5 major features. Work with engineering team of 8."
        },
        {
            "title": "Associate Product Manager",
            "company": companies[1],
            "duration": "2018-2020",
            "description": "Managed analytics dashboard product. Conducted user interviews and A/B tests. Increased user engagement 40%."
        }
    ]


def generate_weak_match_history():
    """Generate work history for weak match (adjacent roles)."""
    roles = [
        ("Technical Project Manager", "coordinate engineering sprints", "20 engineers"),
        ("Senior Business Analyst", "gather requirements and document specs", "stakeholders"),
        ("Program Manager", "oversee portfolio of technical initiatives", "multiple teams")
    ]

    role_type = random.choice(roles)
    companies = ["Tech Solutions", "Enterprise Systems", "Digital Services"]

    return [
        {
            "title": role_type[0],
            "company": companies[0],
            "duration": "2018-Present",
            "description": f"Lead technical projects and {role_type[1]}. Work with {role_type[2]} on delivery timelines."
        },
        {
            "title": "Project Coordinator",
            "company": companies[1],
            "duration": "2015-2018",
            "description": "Supported project managers on enterprise software implementations. Created project plans and tracked milestones."
        }
    ]


def generate_poor_match_history():
    """Generate work history for poor match (unrelated roles)."""
    role_types = [
        ("Software Engineer", "Built web applications", "React, Node.js, PostgreSQL"),
        ("Product Designer", "Designed user interfaces", "Figma, user research"),
        ("Data Scientist", "Analyzed product metrics", "Python, SQL, machine learning")
    ]

    role_type = random.choice(role_types)
    companies = ["StartupCo", "TechVentures", "Innovation Labs"]

    return [
        {
            "title": role_type[0],
            "company": companies[0],
            "duration": "2019-Present",
            "description": f"{role_type[1]} using {role_type[2]}."
        },
        {
            "title": f"Junior {role_type[0]}",
            "company": companies[1],
            "duration": "2017-2019",
            "description": f"Supported team in {role_type[1].lower()}."
        }
    ]


def format_resume(name, summary, work_history, education, skills):
    """Format resume content as text."""
    content = f"{name}\n"
    content += f"Email: {name.lower().replace(' ', '.')}@email.com\n"
    content += f"Phone: +1-555-{random.randint(100, 999)}-{random.randint(1000, 9999)}\n\n"

    content += f"SUMMARY\n{summary}\n\n"

    content += "EXPERIENCE\n"
    for job in work_history:
        content += f"\n{job['title']}\n"
        content += f"{job['company']} | {job['duration']}\n"
        content += f"{job['description']}\n"

    content += f"\nEDUCATION\n{education}\n\n"

    content += f"SKILLS\n{', '.join(skills)}\n"

    return content


def generate_security_documents():
    """Generate vulnerability scan and asset inventory."""
    docs = []

    # Generate vulnerability scan report
    vulns = generate_vulnerabilities()
    vuln_content = "# Vulnerability Scan Report\n\n"
    vuln_content += f"Scan Date: {datetime.now().strftime('%Y-%m-%d')}\n"
    vuln_content += f"Total Findings: {len(vulns)}\n\n"
    vuln_content += "| CVE ID | Severity | Host | Description | Discovery Date |\n"
    vuln_content += "|--------|----------|------|-------------|----------------|\n"

    for vuln in vulns:
        vuln_content += f"| {vuln['cve']} | {vuln['severity']} | {vuln['host']} | {vuln['description']} | {vuln['date']} |\n"

    docs.append({
        "id": "DOC-VULN001",
        "title": "Q1 2026 Vulnerability Scan Report",
        "sensitivity": "confidential",
        "required_department": "security",
        "content": vuln_content
    })

    # Generate asset inventory
    assets = generate_asset_inventory()
    asset_content = "# Asset Inventory\n\n"
    asset_content += "| Host | Team | SLA Tier | SLA Deadline |\n"
    asset_content += "|------|------|----------|-------------|\n"

    for asset in assets:
        asset_content += f"| {asset['host']} | {asset['team']} | {asset['tier']} | {asset['sla']} |\n"

    docs.append({
        "id": "DOC-ASSET001",
        "title": "Infrastructure Asset Inventory",
        "sensitivity": "internal",
        "required_department": "security",
        "content": asset_content
    })

    return docs


def generate_vulnerabilities():
    """Generate 50 vulnerability findings."""
    vulns = []
    hosts = [
        "web-prod-01.example.com", "web-prod-02.example.com",
        "api-prod-01.example.com", "api-prod-02.example.com",
        "db-prod-01.example.com", "db-prod-02.example.com",
        "cache-prod-01.example.com", "cache-prod-02.example.com"
    ]

    severities = ["critical", "high", "medium", "low"]
    severity_weights = [5, 15, 20, 60]  # Realistic distribution

    vuln_templates = [
        ("OpenSSL", "Outdated OpenSSL version with known vulnerabilities"),
        ("nginx", "nginx server vulnerable to request smuggling"),
        ("PostgreSQL", "Database allows weak authentication methods"),
        ("Redis", "Redis instance accessible without authentication"),
        ("Docker", "Container runtime has privilege escalation vulnerability"),
        ("SSH", "SSH server allows weak key exchange algorithms"),
        ("TLS", "TLS configuration supports deprecated cipher suites"),
        ("kernel", "Linux kernel vulnerable to local privilege escalation"),
        ("Python", "Python interpreter has arbitrary code execution flaw"),
        ("Node.js", "Node.js runtime has prototype pollution vulnerability")
    ]

    base_date = datetime.now() - timedelta(days=30)

    for i in range(50):
        severity = random.choices(severities, weights=severity_weights)[0]
        component, desc = random.choice(vuln_templates)

        vulns.append({
            "cve": f"CVE-2026-{random.randint(10000, 99999)}",
            "severity": severity,
            "host": random.choice(hosts),
            "description": desc,
            "date": (base_date + timedelta(days=random.randint(0, 30))).strftime("%Y-%m-%d")
        })

    return vulns


def generate_asset_inventory():
    """Generate asset inventory with SLA tiers."""
    assets = []
    hosts = [
        "web-prod-01.example.com", "web-prod-02.example.com",
        "api-prod-01.example.com", "api-prod-02.example.com",
        "db-prod-01.example.com", "db-prod-02.example.com",
        "cache-prod-01.example.com", "cache-prod-02.example.com"
    ]

    teams = ["Platform", "API", "Data", "Infrastructure"]

    now = datetime.now()

    for host in hosts:
        if "web-prod" in host or "api-prod" in host:
            tier = "Tier 1"
            sla_hours = 24
        elif "db-prod" in host:
            tier = "Tier 2"
            sla_hours = 24 * 7
        else:
            tier = "Tier 3"
            sla_hours = 24 * 30

        # Some assets past SLA deadline
        if random.random() < 0.3:
            sla_date = now - timedelta(hours=sla_hours + random.randint(1, 48))
        else:
            sla_date = now + timedelta(hours=random.randint(1, sla_hours))

        assets.append({
            "host": host,
            "team": random.choice(teams),
            "tier": tier,
            "sla": sla_date.strftime("%Y-%m-%d")
        })

    return assets


def generate_finance_documents():
    """Generate contracts and invoices with planted anomalies."""
    docs = []

    vendors = {
        "V001": "CloudCompute Inc",
        "V002": "StorageMax Solutions",
        "V003": "BandwidthPro",
        "V004": "SupportFirst",
        "V005": "DataCenter Co"
    }

    # Generate 5 contracts
    base_rates = {
        "V001": {"compute": 0.10, "storage": 0.02, "bandwidth": 0.08, "support": 5000},
        "V002": {"compute": 0.12, "storage": 0.015, "bandwidth": 0.10, "support": 4500},
        "V003": {"compute": 0.11, "storage": 0.018, "bandwidth": 0.07, "support": 5500},
        "V004": {"compute": 0.09, "storage": 0.022, "bandwidth": 0.09, "support": 4000},
        "V005": {"compute": 0.13, "storage": 0.016, "bandwidth": 0.11, "support": 6000}
    }

    for vendor_id, vendor_name in vendors.items():
        rates = base_rates[vendor_id]
        content = f"# SERVICE AGREEMENT\n\n"
        content += f"Vendor: {vendor_name}\n"
        content += f"Contract ID: DOC-CTR-{vendor_id}\n"
        content += f"Effective Date: 2026-01-01\n"
        content += f"Term: 12 months\n\n"
        content += "## PRICING\n\n"
        content += f"- Compute: ${rates['compute']}/hour\n"
        content += f"- Storage: ${rates['storage']}/GB/month\n"
        content += f"- Bandwidth: ${rates['bandwidth']}/GB\n"
        content += f"- Support: ${rates['support']}/month\n"

        docs.append({
            "id": f"DOC-CTR-{vendor_id}",
            "title": f"Service Agreement - {vendor_name}",
            "sensitivity": "confidential",
            "required_department": "finance",
            "content": content
        })

    # Generate invoices for 3 months (Jan, Feb, Mar)
    months = ["2026-01", "2026-02", "2026-03"]

    for month in months:
        for vendor_id, vendor_name in vendors.items():
            invoice_num = f"{vendor_id}-{month}"
            rates = base_rates[vendor_id].copy()

            # Plant anomalies
            # V002: 30% compute rate increase in March
            if vendor_id == "V002" and month == "2026-03":
                rates["compute"] = rates["compute"] * 1.30

            # V004: duplicate support charge in February
            duplicate_support = vendor_id == "V004" and month == "2026-02"

            # V003: uncontracted consulting in January
            extra_consulting = vendor_id == "V003" and month == "2026-01"

            # Calculate amounts
            compute_hours = random.randint(8000, 12000)
            storage_gb = random.randint(50000, 80000)
            bandwidth_gb = random.randint(20000, 40000)

            compute_amt = compute_hours * rates["compute"]
            storage_amt = storage_gb * rates["storage"]
            bandwidth_amt = bandwidth_gb * rates["bandwidth"]
            support_amt = rates["support"]

            content = f"# INVOICE\n\n"
            content += f"Vendor: {vendor_name}\n"
            content += f"Invoice Number: {invoice_num}\n"
            content += f"Period: {month}\n\n"
            content += "## LINE ITEMS\n\n"
            content += f"- Compute ({compute_hours} hours @ ${rates['compute']}/hour): ${compute_amt:.2f}\n"
            content += f"- Storage ({storage_gb} GB @ ${rates['storage']}/GB): ${storage_amt:.2f}\n"
            content += f"- Bandwidth ({bandwidth_gb} GB @ ${rates['bandwidth']}/GB): ${bandwidth_amt:.2f}\n"
            content += f"- Support: ${support_amt:.2f}\n"

            total = compute_amt + storage_amt + bandwidth_amt + support_amt

            if duplicate_support:
                content += f"- Support (additional): ${support_amt:.2f}\n"
                total += support_amt

            if extra_consulting:
                consulting_amt = 15000
                content += f"- Consulting Services: ${consulting_amt:.2f}\n"
                total += consulting_amt

            content += f"\n**TOTAL: ${total:.2f}**\n"

            docs.append({
                "id": f"DOC-INV-{invoice_num}",
                "title": f"Invoice {invoice_num} - {vendor_name}",
                "sensitivity": "confidential",
                "required_department": "finance",
                "content": content
            })

    return docs


if __name__ == "__main__":
    main()
